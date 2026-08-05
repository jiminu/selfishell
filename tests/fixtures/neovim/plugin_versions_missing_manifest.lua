local ok, versions = pcall(require, "config.plugin_versions")
assert(ok, "config.plugin_versions must not error when the manifest is missing: " .. tostring(versions))
assert(versions.revision("folke/lazy.nvim") == nil,
  "revision() should return nil for any plugin when the manifest is missing")

local spec_ok, spec_err = pcall(versions.spec, "folke/lazy.nvim")
assert(not spec_ok, "spec() should still fail for a plugin with no approved revision")
assert(tostring(spec_err):match("Missing approved Neovim plugin revision"),
  "spec() failure should stay the clear 'missing approved revision' message, got: " .. tostring(spec_err))

print("plugin_versions missing manifest: OK")
