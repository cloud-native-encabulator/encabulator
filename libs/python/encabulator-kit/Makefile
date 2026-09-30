# encabulator-kit: the shared Python service kit.
.PHONY: build test test-unit lint fmt run clean

build:
	uv sync
	uv build

test:
	uv run pytest -q

test-unit:
	uv run pytest -q -m "not integration"

lint:
	uv run ruff check .
	uv run ruff format --check .

fmt:
	uv run ruff format .
	uv run ruff check --fix .

# The kit is a library: "running" it means proving it imports and reporting its version.
run:
	uv run python -c "import encabulator_kit as k; print(k.__doc__.splitlines()[0]); print('version', k.__version__)"

clean:
	rm -rf .pytest_cache .ruff_cache dist
