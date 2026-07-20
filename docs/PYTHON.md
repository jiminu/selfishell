# Python Development with Selfishell

Selfishell manages Python runtimes and packages using a modern, fast toolchain powered by **`mise`** and **`uv`**. This workflow ensures high performance, reproducibility, and clean environment isolation.

## Toolchain Overview

* **Runtime Manager (`mise`)**: Handles the installation of global and local Python versions.
* **Package & Virtualenv Manager (`uv`)**: Handles dependencies, virtual environments, and project bootstrapping at near-instant speed.
* **Auto-Activation**: When configured, entering a project directory with a `.venv` directory will automatically source and activate it.

---

## Getting Started

### 1. Creating a Virtual Environment

Navigate to your Python project directory and run `uv venv` to create a virtual environment. It will be created in a `.venv` folder by default.

```bash
cd /path/to/project
uv venv
```

If you need a specific Python version, you can define it during creation:

```bash
uv venv --python 3.12
```

### 2. Auto-Activation

To enable auto-activation, add `python.uv_venv_auto = "create|source"` to the `[settings]` section of your project's local `mise.toml` file:

```toml
[settings]
python.uv_venv_auto = "create|source"
```

Once configured, simply entering the directory will activate it:

```bash
cd /path/to/project
# Your shell prompt (Starship) will show the active virtual environment (.venv)
# Run which to verify
which python
# Should output: /path/to/project/.venv/bin/python
```

### 3. Installing Packages

Use `uv pip` to install packages inside the active virtual environment at high speeds:

```bash
uv pip install requests
```

To install from a `requirements.txt` file:

```bash
uv pip install -r requirements.txt
```

To generate a pinned lock file from dependency specifications:

```bash
uv pip compile pyproject.toml -o requirements.txt
```

---

## Editor Integration (Neovim)

Selfishell's built-in Neovim configuration integrates with Python LSP and tools. 

To ensure Neovim can resolve your project dependencies, always run `neovim` from the project root after the virtual environment has been created and packages have been installed.
Once auto-activation is configured, `mise` will automatically activate `.venv` when entering the directory, allowing Neovim to inherit the correct path to the local virtualenv Python interpreter.
