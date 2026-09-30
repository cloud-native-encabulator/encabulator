from __future__ import annotations

import json
import logging

from encabulator_kit.context import reset_request_id, set_request_id
from encabulator_kit.logs import JsonFormatter, configure_logging


def _record(**kwargs: object) -> logging.LogRecord:
    record = logging.LogRecord(
        name="test",
        level=logging.INFO,
        pathname=__file__,
        lineno=1,
        msg="surmounted the %s casing",
        args=("malleable logarithmic",),
        exc_info=None,
    )
    for key, value in kwargs.items():
        setattr(record, key, value)
    return record


def test_required_fields() -> None:
    payload = json.loads(JsonFormatter("side-fumbling-detector").format(_record()))
    assert payload["msg"] == "surmounted the malleable logarithmic casing"
    assert payload["level"] == "info"
    assert payload["service"] == "side-fumbling-detector"
    # RFC 3339 with milliseconds.
    assert payload["time"].endswith("Z")
    assert len(payload["time"].split(".")[1]) == 4  # "123Z"


def test_extras_become_top_level_fields() -> None:
    payload = json.loads(JsonFormatter("svc").format(_record(order_id="ord_1", attempt=2)))
    assert payload["order_id"] == "ord_1"
    assert payload["attempt"] == 2


def test_request_id_included_when_set() -> None:
    token = set_request_id("req-42")
    try:
        payload = json.loads(JsonFormatter("svc").format(_record()))
    finally:
        reset_request_id(token)
    assert payload["request_id"] == "req-42"


def test_no_trace_id_without_tracing() -> None:
    payload = json.loads(JsonFormatter("svc").format(_record()))
    assert "trace_id" not in payload


def test_exception_is_rendered_as_error_field() -> None:
    try:
        raise ValueError("side fumbling")
    except ValueError:
        import sys

        record = _record()
        record.exc_info = sys.exc_info()
    payload = json.loads(JsonFormatter("svc").format(record))
    assert "ValueError: side fumbling" in payload["error"]


def test_configure_logging_is_json_on_stdout(capsys) -> None:
    configure_logging("unilateral-phase-detractor", "debug")
    logging.getLogger("x").debug("inverse reactive current", extra={"kid": "k1"})
    line = capsys.readouterr().out.strip().splitlines()[-1]
    payload = json.loads(line)
    assert payload["service"] == "unilateral-phase-detractor"
    assert payload["level"] == "debug"
    assert payload["kid"] == "k1"
