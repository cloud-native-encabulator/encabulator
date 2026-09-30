"""Request and response models. ``roles`` is always an array on the wire."""

from __future__ import annotations

from pydantic import BaseModel, ConfigDict, Field


class LoginRequest(BaseModel):
    model_config = ConfigDict(extra="forbid")

    username: str = Field(min_length=1, max_length=64)
    password: str = Field(min_length=1, max_length=1024)


class RegisterRequest(BaseModel):
    model_config = ConfigDict(extra="forbid")

    username: str = Field(min_length=1, max_length=64)
    password: str = Field(min_length=1, max_length=1024)
    displayName: str = Field(default="", max_length=128)  # noqa: N815 - wire format is camelCase


class CreateUserRequest(BaseModel):
    model_config = ConfigDict(extra="forbid")

    username: str = Field(min_length=1, max_length=64)
    password: str = Field(min_length=1, max_length=1024)
    displayName: str = Field(default="", max_length=128)  # noqa: N815
    roles: list[str] = Field(default_factory=lambda: ["customer"])


class PatchUserRequest(BaseModel):
    model_config = ConfigDict(extra="forbid")

    password: str | None = Field(default=None, min_length=1, max_length=1024)
    displayName: str | None = Field(default=None, max_length=128)  # noqa: N815
    roles: list[str] | None = None
    disabled: bool | None = None
