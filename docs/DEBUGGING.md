# Debugging in Neovim

Selfishell includes nvim-dap, nvim-dap-ui, nvim-nio and mason-nvim-dap at
approved commits. They load on the first debug key or DAP command. See the
[Neovim workflow](ENVIRONMENT.md#neovim-workflow) for general editor navigation.

## Install adapters

Install only the adapters you need, then open Neovim from the project root:

```vim
:DapInstall python
:DapInstall delve
:DapInstall js
:DapInstall codelldb
```

These install Python's debugpy, Go's Delve, the JavaScript/TypeScript debugger,
and CodeLLDB for C/C++ and Rust respectively. The language runtime/toolchain
must also be installed.
Mason installs supported adapters
and the bridge registers their available default launch configurations,
including installations completed during the current Neovim session.
`:MasonInstall debugpy`, `:MasonInstall delve` and
`:MasonInstall js-debug-adapter` also work. No debug adapter
is installed merely by opening Neovim or running Selfishell setup.
When F5 or `Space D c` finds no launch configuration, Selfishell shows the
installation command for supported defaults if the adapter is missing.
If an adapter is already installed or registered, it points to launch
configuration help instead. Personal Lua providers and `launch.json` are
checked before showing this guidance.

## Start and control a session

Save the source files with `:w` (or `:wa` for all edited files) before starting:
debuggers run the files on disk, so unsaved edits can leave breakpoints out of
sync. New launches warn about unsaved changes in the current file and other
loaded files under the working directory. The warning leaves your edits
unsaved and lets the launch proceed; save and restart to debug the new code.
It does not repeat when continuing a paused session or attaching to a process.
In Python, launching an empty file with unsaved code can report
`line 0` / `Invalid cursor line`; terminate the session, save, and launch again.
Set a breakpoint with `F9` (a red circle in the gutter), then press `F5` and
select a launch configuration.
Debug panels open once the session initializes and close when the last session
closes, including disconnects and adapter failures. The file explorer closes
when the debug panels open for a session; use `Space e` to reopen it when needed.
Toggling the explorer preserves the debug panel sizes, including manual resizing.
Use Normal mode for the following keys:

| Action | Function key | Space shortcut |
| --- | --- | --- |
| Start / continue | `F5` | `Space D c` |
| Toggle breakpoint | `F9` | `Space D b` |
| Step over | `F10` | `Space D o` |
| Step into | `F11` | `Space D i` |
| Step out | `Shift+F11` | `Space D O` |
| Terminate | `Shift+F5` | `Space D t` |
| Pause execution | | `Space D p` |
| Go to current frame | | `Space D f` |
| Toggle debug panels | | `Space D u` |
| Evaluate expression | | `Space D e` (also Visual mode) |

The Space shortcuts also work when a keyboard or terminal intercepts function
keys. Which-key shows the actions and their function-key equivalents. `Space d`
still shows LSP diagnostics; `Ctrl+h/j/k/l` moves between debug windows.
Stepping follows the execution line in a source window, including after moving
between debug panels. The REPL keeps input focus while the source view follows.
Use `Space D p` (or `:DapPause`) during a long-running operation to inspect where
it is executing; adapters may ask which thread to pause. `Space D f` returns
the source view to the selected stack frame after browsing elsewhere.
`No stopped threads. Cannot move` means the program is running: wait for it to
stop, or pause it before using F10/F11.
`Space D u` hides all remaining debug panels, even if some were closed with
`:q`; when all panels are hidden, it opens the complete layout.

### Debug panels and source windows

`:q` closes a window while its source buffer can remain loaded. If all source
windows were closed, terminating the session or hiding panels with `Space D u`
restores an editor for the source buffer. A deleted buffer is not reopened;
an empty editor is created instead.
In Breakpoints or Stacks, `o` opens the selected location, recreating an editor
if needed. Panel navigation reuses an available editor; use a personal
dap-ui `select_window` option to choose a different window-selection policy.

### Function keys in terminals

Terminals that report `Shift+F5` / `Shift+F11` as `F17` / `F23` are supported.
Windows Terminal binds F11 to full screen by default. Remove or change that
binding in Settings > Actions to pass F11 to Neovim, or use `Space D i`.
See [Windows Terminal actions](https://learn.microsoft.com/en-us/windows/terminal/customize-settings/actions#toggle-full-screen).
On a Mac keyboard, use `Fn` (or Globe) with a function key, or enable standard
function keys in Keyboard settings; macOS shortcuts such as Show Desktop can
also intercept F11. See [Apple's function-key guide](https://support.apple.com/102439).

## Python and Go

Python's default launch runs the current file. On each launch, Selfishell
selects an executable from `VIRTUAL_ENV`, then `CONDA_PREFIX`, then the current
project's `.venv/bin/python`, then `python3` or `python` on PATH. The adapter's
own Python environment is separate from the interpreter running your program.
Go uses the bridge's package and test launch configurations; run Neovim from
the appropriate project directory or specify the package explicitly.

## C, C++ and Rust

For C/C++, build with debug information, then use `LLDB: Launch` and select the
executable. For a CMake project using a single-configuration generator:

```sh
cmake -S . -B build/debug -DCMAKE_BUILD_TYPE=Debug
cmake --build build/debug
```

Use the project's existing Debug preset when provided. With a multi-configuration
generator, select Debug at build time with `--config Debug`. F5 launches the
existing executable; rebuild after source changes. Rust's `cargo build` normally
produces a debug executable under `target/debug/`.
`LLDB: Launch (args)` and Go's `Delve: Debug (Arguments)` accept quoted arguments,
such as `--label "hello world"`; quotes group one argument and are removed. These
inputs do not run a shell or expand variables. For repeatable runs, set `program`,
`cwd` and an `args` array in the project's launch configuration. Relative data
paths are resolved from `cwd`, which defaults to the project directory.

### Exception breakpoints

CodeLLDB enables C++ throw exception breakpoints by default. It can stop in
`__cxa_throw` even if the application catches the exception and continues.
Code without source debug information is shown as disassembly. To turn off
exception breakpoints for the current session while keeping ordinary breakpoints:

```vim
:lua require("dap").set_exception_breakpoints({})
```

Then press F5 to continue. To restore the adapter's exception defaults in that
session, use `:lua require("dap").set_exception_breakpoints("default")`.
See [CodeLLDB's disassembly documentation](https://github.com/vadimcn/codelldb/blob/master/MANUAL.md#disassembly-view).

## JavaScript and TypeScript

JavaScript and TypeScript share Microsoft's `js-debug-adapter`. Installing
`:DapInstall js` registers the following defaults for both languages:

| Configuration | Use |
| --- | --- |
| `Node: Launch current file` | Run the open file with Node on PATH. |
| `Node: Launch JavaScript file` | Choose a JavaScript entrypoint, including compiled TypeScript output. |
| `Node: Attach (port)` | Connect to a local Node inspector; the prompt defaults to port 9229. |

Both launch options use the debug UI's Console terminal, so programs using
`readline` or other standard input work. Move to Console and press `i` to
enter input. Press `Ctrl+\` then `Ctrl+n` to return to Normal mode before
using debug shortcuts. Attach keeps the process's existing terminal.

Selfishell's Node 26 can run `.ts`, `.mts` and `.cts` files with erasable type
annotations directly. This does not type-check or apply `tsconfig.json`;
enums, decorators, JSX/TSX and path aliases may need your project's compiler
or runtime. For compiled TypeScript, enable source maps in the project build,
open the original `.ts` file, set a breakpoint and choose
`Node: Launch JavaScript file`. Select the generated `.js`, `.mjs` or `.cjs`
entrypoint. Source maps are enabled, and generated files are searched under
the working directory, excluding `node_modules`. Run the project's build
again after edits; F5 does not build automatically.
For a project that already uses `tsx`, a project launch configuration can
set `"runtimeArgs": ["--import", "tsx"]` to run its TypeScript entrypoint.
Selfishell does not install project runtimes or build dependencies.

For attach, start your program with `node --inspect-brk=127.0.0.1:9229 app.js`
(or the equivalent flags in your project command), then select
`Node: Attach (port)`. Node may pause at entry; F5 continues to your breakpoint.
Framework launches use project-specific configuration. Browser debugging
also needs a browser adapter registration; these defaults target Node.js.
See [Node's TypeScript support](https://nodejs.org/docs/latest-v24.x/api/typescript.html)
and [js-debug options](https://github.com/microsoft/vscode-js-debug/blob/main/OPTIONS.md).

## Project launch configuration

For project-specific entrypoints, arguments, environment variables or attach
settings, use `.vscode/launch.json`. nvim-dap reads it when starting a session;
its `type` must match a registered adapter (for these defaults, `python`,
`delve`, `codelldb`, or `pwa-node` / `node` for JS/TS). Only a subset of VS Code's format
is supported; use standard JSON without trailing commas. See `:help dap-launch.json`.

Adapter installation does not guarantee a default launch configuration for
every language. Java, for example, needs JDTLS plus `java-debug-adapter` and
additional JDTLS/DAP integration, commonly through
[nvim-jdtls](https://github.com/mfussenegger/nvim-jdtls#debugger-via-nvim-dap).
`:DapInstall javadbg` installs that package; it does not complete the integration.
Selfishell does not configure Java or framework-specific launch workflows.

## Personal adapter settings

For personal adapter settings, add `~/.config/nvim/lua/plugins/dap_user.lua`
(under `$XDG_CONFIG_HOME` when set). Existing lazy.nvim imports load it;
Selfishell does not create, replace or remove this file. For example:

```lua
return {
  {
    "jay-babu/mason-nvim-dap.nvim",
    opts = function(_, opts)
      opts.handlers.python = function(config)
        config.configurations[1].pythonPath = "/absolute/path/to/venv/bin/python"
        require("mason-nvim-dap").default_setup(config)
      end
    end,
  },
}
```

Use overrides for the included plugins; installing additional Neovim plugins
into the managed plugin directory is subject to
[plugin synchronization](UPDATES.md). Adapter updates and removal remain under
`:Mason` control; see [debug adapter updates](UPDATES.md#neovim-debug-adapters).
