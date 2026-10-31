-- Run as `nvim --clean --headless -l scripts/check-lua.lua FILE...` (scripts/check-lua.sh).
-- loadfile only compiles, so configuration code never runs.
local failed = 0
for _, file in ipairs(arg) do
  local _, err = loadfile(file)
  if err then
    io.stderr:write(err, '\n')
    failed = failed + 1
  end
end
if failed > 0 then
  io.stderr:write(failed, ' Lua file(s) failed to parse\n')
  os.exit(1)
end
