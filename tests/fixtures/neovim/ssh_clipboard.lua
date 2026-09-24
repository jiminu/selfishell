-- Over SSH and on WSL, copies go to the local terminal with OSC 52 and pastes
-- return the last copy without asking the terminal; locally, Neovim picks.
local real_has = vim.fn.has
local function load_options(wsl)
  package.loaded["config.options"] = nil
  vim.g.clipboard = nil
  vim.fn.has = function(feature)
    if feature == "wsl" then
      return wsl and 1 or 0
    end
    return real_has(feature)
  end
  require("config.options")
  vim.fn.has = real_has
end

vim.env.SSH_TTY = nil
vim.env.SSH_CONNECTION = nil
load_options(false)
assert(vim.g.clipboard == nil, "a local session overrode the clipboard provider")

load_options(true)
assert(vim.g.clipboard, "WSL kept Neovim's clipboard provider search, which finds none")

vim.env.SSH_CONNECTION = "192.0.2.1 50000 192.0.2.2 22"
load_options(false)
local clipboard = assert(vim.g.clipboard, "an SSH session kept the remote clipboard provider")

local sent = {}
vim.api.nvim_ui_send = function(sequence)
  sent[#sent + 1] = sequence
end
clipboard.copy["+"]({ "first", "second" }, "V")
assert(
  sent[1] == "\027]52;c;" .. vim.base64.encode("first\nsecond") .. "\027\\",
  "the copy was not sent with OSC 52: " .. vim.inspect(sent)
)
assert(
  vim.deep_equal(clipboard.paste["+"](), { { "first", "second" }, "V" }),
  "paste did not return the last copy: " .. vim.inspect(clipboard.paste["+"]())
)
assert(#sent == 1, "paste queried the terminal: " .. vim.inspect(sent))

print("SSH clipboard: OK")
