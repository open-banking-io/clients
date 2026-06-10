import json
from pathlib import Path

import pytest

# python/tests/ -> python/ -> repo root -> fixtures/
FIXTURES = Path(__file__).resolve().parents[2] / "fixtures"


def load_fixture(*parts: str) -> dict | list:
    with open(FIXTURES.joinpath(*parts), encoding="utf-8") as fh:
        return json.load(fh)


@pytest.fixture
def fixtures_dir() -> Path:
    return FIXTURES


@pytest.fixture
def keypair() -> dict:
    return load_fixture("keypair.json")


@pytest.fixture
def credentials() -> dict:
    return load_fixture("credentials.json")


@pytest.fixture
def envelopes() -> dict:
    return load_fixture("envelopes.json")


@pytest.fixture
def expected() -> dict:
    return load_fixture("expected.json")
