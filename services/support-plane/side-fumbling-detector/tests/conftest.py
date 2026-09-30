from __future__ import annotations

import pytest
from encabulator_kit.testing import container_runtime_available


def pytest_configure(config: pytest.Config) -> None:
    config.addinivalue_line("markers", "integration: needs a container runtime")


def pytest_collection_modifyitems(config: pytest.Config, items: list[pytest.Item]) -> None:
    if container_runtime_available():
        return
    skip = pytest.mark.skip(reason="no container runtime (DOCKER_HOST unset); see dev-env.sh")
    for item in items:
        if "integration" in item.keywords:
            item.add_marker(skip)
