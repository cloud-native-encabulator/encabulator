from __future__ import annotations

from encabulator_kit.errors import DEFAULT_ERROR_CODES, EncabulatorError, UpstreamError, error_body


def test_error_body_shape() -> None:
    assert error_body("insufficient_parts", "no amulite") == {
        "error": {"code": "insufficient_parts", "message": "no amulite"}
    }


def test_default_code_from_status() -> None:
    error = EncabulatorError(409)
    assert error.code == "conflict"
    assert error.body()["error"]["code"] == "conflict"


def test_details_are_merged_into_the_error_object() -> None:
    error = EncabulatorError(409, "insufficient_parts", "short", details={"skus": ["HYD-MARZ"]})
    assert error.body()["error"]["skus"] == ["HYD-MARZ"]


def test_codes_cover_the_statuses_the_design_uses() -> None:
    for status in (400, 401, 403, 404, 409, 429, 500, 503):
        assert status in DEFAULT_ERROR_CODES


def test_upstream_error_message() -> None:
    error = UpstreamError("spurving-bearing-quoter", 503, "unavailable", "down")
    assert "spurving-bearing-quoter responded 503" in str(error)
