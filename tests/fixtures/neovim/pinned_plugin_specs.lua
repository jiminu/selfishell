local modules = {
  "plugins.completion",
  "plugins.editor",
  "plugins.lsp",
  "plugins.telescope",
  "plugins.ui",
}

local specs = {}

for _, module in ipairs(modules) do
  specs[module] = require(module)
  assert(type(specs[module]) == "table", "Invalid plugin spec: " .. module)
end

local indent_blankline = vim.tbl_filter(function(spec)
  return spec[1] == "lukas-reineke/indent-blankline.nvim"
end, specs["plugins.ui"])[1]

assert(indent_blankline.opts.indent.char == " ", "Indent guides must reserve space for the scope marker")
assert(indent_blankline.opts.scope.char == "│", "Scope marker must use a thin solid line")
assert(indent_blankline.opts.scope.show_start == false, "Scope start underline must stay disabled")
assert(indent_blankline.opts.scope.show_end == false, "Scope end underline must stay disabled")
assert(vim.deep_equal(indent_blankline.opts.scope.highlight, {
  "RainbowDelimiterRed",
  "RainbowDelimiterYellow",
  "RainbowDelimiterBlue",
  "RainbowDelimiterOrange",
  "RainbowDelimiterGreen",
  "RainbowDelimiterViolet",
  "RainbowDelimiterCyan",
}), "Scope must use the rainbow delimiter palette")

local hook_registration
package.preload["ibl"] = function()
  return {
    setup = function() end,
  }
end
package.preload["ibl.hooks"] = function()
  return {
    type = { SCOPE_HIGHLIGHT = "scope-highlight" },
    builtin = { scope_highlight_from_extmark = "rainbow-extmark" },
    register = function(kind, callback)
      hook_registration = { kind, callback }
    end,
  }
end

assert(type(indent_blankline.config) == "function", "Scope color sync must register a hook")
indent_blankline.config(nil, indent_blankline.opts)
assert(vim.deep_equal(hook_registration, { "scope-highlight", "rainbow-extmark" }),
  "Scope must follow the rainbow delimiter extmark color")

print("pinned plugin specs: OK")
