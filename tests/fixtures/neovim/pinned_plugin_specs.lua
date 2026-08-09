local modules = {
  "plugins.editor",
  "plugins.lsp",
  "plugins.ui",
}

local specs = {}

for _, module in ipairs(modules) do
  specs[module] = require(module)
  assert(type(specs[module]) == "table", "Invalid plugin spec: " .. module)
end

local snacks = vim.tbl_filter(function(spec)
  return spec[1] == "folke/snacks.nvim"
end, specs["plugins.ui"])[1]

assert(snacks, "folke/snacks.nvim spec was not found")
assert(snacks.lazy == false, "Snacks must load eagerly so it can attach to the first buffer's BufReadPost")
assert(type(snacks.config) == "function",
  "Snacks must explicitly enable indent after setup, since Snacks never binds it to BufNewFile")

local indent = snacks.opts.indent
assert(indent.enabled == true, "Snacks indent module must be enabled")
assert(indent.indent.enabled == false, "Normal indent rendering must stay disabled")
assert(indent.indent.only_scope == true, "Indent rendering must be limited to the current scope")
assert(indent.animate.enabled == false, "Scope animation must stay disabled")
assert(indent.chunk.enabled == false, "Chunk rendering must stay disabled")

assert(indent.scope.enabled == true, "Scope rendering must be enabled")
assert(indent.scope.char == "│", "Scope marker must use a thin solid line")
assert(indent.scope.underline == false, "Scope start underline must stay disabled")
assert(vim.deep_equal(indent.scope.hl, {
  "RainbowDelimiterRed",
  "RainbowDelimiterYellow",
  "RainbowDelimiterBlue",
  "RainbowDelimiterOrange",
  "RainbowDelimiterGreen",
  "RainbowDelimiterViolet",
  "RainbowDelimiterCyan",
}), "Scope must use the rainbow delimiter palette")

assert(indent.scope.treesitter.enabled == true, "Scope detection must prefer Tree-sitter")
assert(type(indent.scope.treesitter.blocks) == "table" and indent.scope.treesitter.blocks.enabled == false,
  "Scope detection must not be limited to a Tree-sitter node-type whitelist")

assert(type(indent.filter) == "function", "Indent rendering must filter excluded buffers")
assert(type(indent.scope.filter) == "function", "Scope detection must filter excluded buffers")

local excluded = { NvimTree = true, help = true, markdown = true }
for filetype in pairs(excluded) do
  vim.bo.filetype = filetype
  assert(indent.filter(0) == false, "Indent rendering did not exclude filetype: " .. filetype)
  assert(indent.scope.filter(0) == false, "Scope detection did not exclude filetype: " .. filetype)
end
vim.bo.filetype = "lua"
assert(indent.filter(0) == true, "Indent rendering incorrectly excluded an ordinary buffer")
assert(indent.scope.filter(0) == true, "Scope detection incorrectly excluded an ordinary buffer")

local picker = snacks.opts.picker
assert(picker, "Snacks picker must be configured")
assert(picker.ui_select == false, "Snacks must not take over vim.ui.select")
assert(picker.sources.files.cmd == "rg", "The files picker must not depend on a personally-installed fd")
assert(picker.sources.files.icons.files.enabled == false, "The files picker should hide the leading file icon")
assert(picker.sources.grep.icons.files.enabled == false, "The grep picker should hide the leading file icon")
assert(picker.sources.buffers.icons.files.enabled == false, "The buffers picker should hide the leading file icon")
assert(picker.sources.diagnostics.filter.cwd == false, "Diagnostics must not be limited to the cwd")

print("pinned plugin specs: OK")
