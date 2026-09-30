# Python Development with Selfishell

Selfishell uses `mise` for Python versions and `uv` for packages and virtual
environments. Project settings can enable automatic virtualenv activation for
uv projects.

## Getting Started

### 1. Creating a uv Project

Create a uv project, or use an existing one with a `pyproject.toml`:

```bash
cd /path/to/project
uv init --python 3.12   # Skip when pyproject.toml already exists.
uv add requests
```

`uv add` records the dependency in `pyproject.toml`, writes `uv.lock`, and
installs it into the project's `.venv`. In an existing project, `uv sync`
creates `.venv` and `uv.lock` from `pyproject.toml`. To import a
`requirements.txt` file:

```bash
uv add -r requirements.txt
```

### 2. Auto-Activation

Automatic activation is mise's `python.uv_venv_auto` setting. It applies only
to a uv project with a `uv.lock` file; a `.venv` directory alone is
insufficient.

Add this setting to your project's `mise.toml`, then run `mise trust` in the
project directory:

```toml
[settings]
python.uv_venv_auto = "create|source"
```

`"source"` activates an existing virtual environment; `"create|source"` also
creates one when necessary.

Once configured, entering the uv project directory activates it:

```bash
cd /path/to/project
which python
# Expected: /path/to/project/.venv/bin/python
```

### 3. Environments without uv.lock

`uv venv` and `uv pip install` create and fill a `.venv` without `uv.lock`, so
auto-activation does not apply. Activate such an environment manually:

```bash
uv venv
uv pip install -r requirements.txt
source .venv/bin/activate
```

## Editor Integration (Neovim)

Create the virtual environment and install dependencies before launching
`nvim` from the project root. With auto-activation configured, Neovim inherits
the uv project's virtualenv Python path for its Python tooling.
