# Python Development with Selfishell

Selfishell uses `mise` for Python versions and `uv` for packages and virtual
environments. Project settings can enable automatic virtualenv activation.

## Getting Started

### 1. Creating a Virtual Environment

Create a `.venv` in your project directory:

```bash
cd /path/to/project
uv venv
```

If you need a specific Python version, you can define it during creation:

```bash
uv venv --python 3.12
```

### 2. Auto-Activation

`python.uv_venv_auto` requires a uv project with a `uv.lock` file, created by
`uv lock` or `uv sync`; a `.venv` directory alone is insufficient.

Add this setting to your project's `mise.toml`:

```toml
[settings]
python.uv_venv_auto = "create|source"
```

`"source"` activates an existing virtual environment; `"create|source"` also
creates one when necessary.

Once configured, entering the uv project directory will activate it:

```bash
cd /path/to/project
which python
# Expected: /path/to/project/.venv/bin/python
```

### 3. Installing Packages

Install packages in the virtual environment:

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

## Editor Integration (Neovim)

Create the virtual environment and install dependencies before launching
`nvim` from the project root. With auto-activation configured, Neovim inherits
the uv project's virtualenv Python path for its Python tooling.
