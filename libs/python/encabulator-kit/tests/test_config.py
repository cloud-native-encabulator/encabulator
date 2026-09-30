from __future__ import annotations

from encabulator_kit.config import CommonSettings, env_bool, env_float, env_int, env_str


def test_env_str_treats_blank_as_unset(monkeypatch) -> None:
    monkeypatch.setenv("ENC_TEST", "   ")
    assert env_str("ENC_TEST", "fallback") == "fallback"
    monkeypatch.setenv("ENC_TEST", " hello ")
    assert env_str("ENC_TEST") == "hello"


def test_numeric_and_boolean_parsing(monkeypatch) -> None:
    monkeypatch.setenv("ENC_INT", "7")
    monkeypatch.setenv("ENC_BAD", "not-a-number")
    monkeypatch.setenv("ENC_FLOAT", "1.5")
    monkeypatch.setenv("ENC_TRUE", "TRUE")
    monkeypatch.setenv("ENC_FALSE", "off")
    assert env_int("ENC_INT", 1) == 7
    assert env_int("ENC_BAD", 1) == 1
    assert env_float("ENC_FLOAT", 0.0) == 1.5
    assert env_bool("ENC_TRUE") is True
    assert env_bool("ENC_FALSE", True) is False
    assert env_bool("ENC_MISSING", True) is True


def test_common_settings_defaults(monkeypatch) -> None:
    for name in (
        "PORT",
        "LOG_LEVEL",
        "OTEL_SERVICE_NAME",
        "OTEL_EXPORTER_OTLP_ENDPOINT",
        "MALAISE_ENGINE_URL",
        "DIFFERENTIAL_GIRDLESPRING_URL",
        "NATS_URL",
    ):
        monkeypatch.delenv(name, raising=False)
    settings = CommonSettings.from_env("side-fumbling-detector")
    assert settings.service_name == "side-fumbling-detector"
    assert settings.port == 8080
    assert settings.log_level == "info"
    assert settings.otlp_endpoint is None
    assert settings.malaise_engine_url is None
    assert settings.girdlespring_url is None
    assert settings.nats_url == "nats://nats:4222"


def test_otel_service_name_wins(monkeypatch) -> None:
    monkeypatch.setenv("OTEL_SERVICE_NAME", "renamed")
    assert CommonSettings.from_env("original").service_name == "renamed"
