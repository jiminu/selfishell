-- A stub source stands in for an LSP so the keys exercise blink.cmp itself.
local listeners = #vim.api.nvim_get_autocmds({ event = "InsertCharPre" })
require("lazy").load({ plugins = { "blink.cmp" } })
-- setup() attaches its typing listeners from a callback once the matcher is ready.
assert(
  vim.wait(5000, function() return #vim.api.nvim_get_autocmds({ event = "InsertCharPre" }) > listeners end),
  "blink.cmp did not attach its typing listeners"
)
local blink = require("blink.cmp")
package.preload["completion_smoke_source"] = function()
  return {
    new = function()
      return {
        get_completions = function(_, _, callback)
          callback({
            is_incomplete_forward = false,
            is_incomplete_backward = false,
            items = {
              { label = "greet", kind = vim.lsp.protocol.CompletionItemKind.Function },
              {
                label = "pair",
                kind = vim.lsp.protocol.CompletionItemKind.Snippet,
                insertText = "pair(${1:first}, ${2:second})",
                insertTextFormat = vim.lsp.protocol.InsertTextFormat.Snippet,
              },
            },
          })
        end,
      }
    end,
  }
end
blink.add_source_provider("completion_smoke", { module = "completion_smoke_source" })
blink.add_filetype_source(vim.bo.filetype, "completion_smoke")

local function line()
  return vim.api.nvim_get_current_line()
end

-- The "x!" feedkeys keeps Insert or Cmdline mode open, running the event loop;
-- each step waits for its condition, then types the next keys from a timer.
local function session(keys, steps)
  local failure
  local index = 0
  local function advance()
    index = index + 1
    local step = steps[index]
    if not step then
      return
    end
    local started = vim.uv.now()
    local timer = vim.uv.new_timer()
    timer:start(20, 20, vim.schedule_wrap(function()
      local elapsed = vim.uv.now() - started
      local ready = step.ready()
      if not ready and elapsed < 5000 then
        return
      end
      timer:stop()
      timer:close()
      if not ready then
        failure = step.failure()
        vim.api.nvim_feedkeys(vim.keycode("<C-c><Esc>"), "n", false)
        return
      end
      vim.api.nvim_feedkeys(vim.keycode(step.keys), "t", false)
      advance()
    end))
  end
  advance()
  vim.api.nvim_feedkeys(vim.keycode(keys), "tx!", false)
  assert(not failure, failure)
end

-- Enter accepts the first item without a selection and adds a function's parentheses.
session("ogre", {
  { ready = blink.is_menu_visible, keys = "<CR>", failure = function() return "menu did not open for 'gre'" end },
  { ready = function() return line() == "greet()" end, keys = "<Esc>", failure = function() return "Enter gave " .. line() end },
})

-- Tab moves to the next snippet field.
session("opai", {
  { ready = blink.is_menu_visible, keys = "<CR>", failure = function() return "menu did not open for 'pai'" end },
  { ready = function() return line() == "pair(first, second)" end, keys = "<Tab>x", failure = function() return "snippet gave " .. line() end },
  { ready = function() return line() == "pair(first, x)" end, keys = "<Esc>", failure = function() return "Tab then x gave " .. line() end },
})

-- ":" commands stay quiet while typing; Tab completes them. Headless Neovim
-- draws no command-line menu, so the quiet part checks the setting itself.
local auto_show = require("blink.cmp.config").cmdline.completion.menu.auto_show
if type(auto_show) == "function" then
  auto_show = auto_show({ mode = "cmdline" }, {})
end
assert(not auto_show, "the command-line menu opens while typing")
local typed = vim.uv.now()
session(":Laz", {
  { ready = function() return vim.uv.now() - typed > 200 end, keys = "<Tab>", failure = function() return "unreachable" end },
  { ready = function() return vim.fn.getcmdline() == "Lazy" end, keys = "<C-c>", failure = function() return "Tab gave " .. vim.fn.getcmdline() end },
})
-- 'confirm' would otherwise offer to save the edited buffer at :qa.
vim.bo.modified = false

print("Completion smoke: OK")
