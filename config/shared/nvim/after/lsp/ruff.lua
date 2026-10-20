-- Pyright owns syntax errors and name resolution for Python; these Ruff
-- reports would repeat its diagnostics.
return {
  init_options = {
    settings = {
      showSyntaxErrors = false,
      lint = {
        -- Undefined names, and undefined names in __all__.
        ignore = { "F821", "F822" },
      },
    },
  },
}
