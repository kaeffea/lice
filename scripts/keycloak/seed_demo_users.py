#!/usr/bin/env python3
"""Create local synthetic users and publish only their immutable subjects."""

from __future__ import annotations

import json
import os
import pathlib
import sys
import urllib.error
import urllib.parse
import urllib.request
import uuid


REALM = "lice"
SUBJECTS_FILE = pathlib.Path("/var/run/lice-bootstrap/subjects.env")
REQUIRED_ENV = (
    "KEYCLOAK_ADMIN_URL",
    "KEYCLOAK_BOOTSTRAP_ADMIN_CLIENT_ID",
    "KEYCLOAK_BOOTSTRAP_ADMIN_CLIENT_SECRET",
    "LICE_DEMO_OPERATOR_USERNAME",
    "LICE_DEMO_OPERATOR_PASSWORD",
    "LICE_DEMO_VIEWER_USERNAME",
    "LICE_DEMO_VIEWER_PASSWORD",
)


def require_environment() -> dict[str, str]:
    values: dict[str, str] = {}
    for name in REQUIRED_ENV:
        value = os.environ.get(name, "")
        if not value:
            raise RuntimeError(f"Variavel obrigatoria ausente no seed: {name}")
        values[name] = value
    return values


def request_json(
    url: str,
    *,
    method: str = "GET",
    token: str | None = None,
    payload: object | None = None,
    form: dict[str, str] | None = None,
    expected: tuple[int, ...] = (200,),
) -> object | None:
    headers = {"Accept": "application/json"}
    body: bytes | None = None

    if token is not None:
        headers["Authorization"] = f"Bearer {token}"
    if payload is not None:
        headers["Content-Type"] = "application/json"
        body = json.dumps(payload, separators=(",", ":")).encode("utf-8")
    elif form is not None:
        headers["Content-Type"] = "application/x-www-form-urlencoded"
        body = urllib.parse.urlencode(form).encode("ascii")

    request = urllib.request.Request(url, data=body, headers=headers, method=method)
    try:
        response = urllib.request.urlopen(request, timeout=15)
    except urllib.error.HTTPError as error:
        endpoint = urllib.parse.urlsplit(url).path
        raise RuntimeError(
            f"Keycloak respondeu HTTP {error.code} em {method} {endpoint}"
        ) from error

    with response:
        if response.status not in expected:
            endpoint = urllib.parse.urlsplit(url).path
            raise RuntimeError(
                f"Resposta inesperada HTTP {response.status} em {method} {endpoint}"
            )
        raw = response.read()
    if not raw:
        return None
    return json.loads(raw)


def issue_token(environment: dict[str, str]) -> str:
    origin = environment["KEYCLOAK_ADMIN_URL"].rstrip("/")
    payload = request_json(
        f"{origin}/realms/master/protocol/openid-connect/token",
        method="POST",
        form={
            "grant_type": "client_credentials",
            "client_id": environment["KEYCLOAK_BOOTSTRAP_ADMIN_CLIENT_ID"],
            "client_secret": environment["KEYCLOAK_BOOTSTRAP_ADMIN_CLIENT_SECRET"],
        },
    )
    if not isinstance(payload, dict) or not isinstance(payload.get("access_token"), str):
        raise RuntimeError("Keycloak nao retornou um access token administrativo.")
    return payload["access_token"]


def canonical_subject(value: object) -> str:
    if not isinstance(value, str):
        raise RuntimeError("Keycloak retornou um subject ausente ou invalido.")
    try:
        parsed = uuid.UUID(value)
    except ValueError as error:
        raise RuntimeError("Keycloak retornou um subject nao UUID.") from error
    canonical = str(parsed)
    if value != canonical:
        raise RuntimeError("Keycloak retornou um subject UUID nao canonico.")
    return canonical


def find_subject(origin: str, token: str, username: str) -> str | None:
    query = urllib.parse.urlencode(
        {"username": username, "exact": "true", "first": "0", "max": "2"}
    )
    payload = request_json(
        f"{origin}/admin/realms/{REALM}/users?{query}", token=token
    )
    if not isinstance(payload, list):
        raise RuntimeError("Keycloak retornou uma busca de usuarios invalida.")

    exact = [item for item in payload if item.get("username") == username]
    if len(exact) > 1:
        raise RuntimeError("Keycloak retornou usuarios sinteticos duplicados.")
    if not exact:
        return None
    return canonical_subject(exact[0].get("id"))


def ensure_user(
    origin: str,
    token: str,
    username: str,
    password: str,
    first_name: str,
    last_name: str,
) -> str:
    profile = {
        "username": username,
        "email": username,
        "firstName": first_name,
        "lastName": last_name,
        "enabled": True,
        "emailVerified": True,
        "requiredActions": [],
    }
    subject = find_subject(origin, token, username)
    if subject is None:
        request_json(
            f"{origin}/admin/realms/{REALM}/users",
            method="POST",
            token=token,
            payload=profile,
            expected=(201,),
        )
        subject = find_subject(origin, token, username)

    if subject is None:
        raise RuntimeError("Nao foi possivel obter o subject do usuario sintetico.")

    # Keep repeated local boots deterministic and repair users created by an
    # older seed that did not populate Keycloak's required profile fields.
    request_json(
        f"{origin}/admin/realms/{REALM}/users/{subject}",
        method="PUT",
        token=token,
        payload=profile,
        expected=(204,),
    )
    request_json(
        f"{origin}/admin/realms/{REALM}/users/{subject}/reset-password",
        method="PUT",
        token=token,
        payload={"type": "password", "value": password, "temporary": False},
        expected=(204,),
    )
    return subject


def publish_subjects(operator_subject: str, viewer_subject: str) -> None:
    SUBJECTS_FILE.parent.mkdir(mode=0o700, parents=True, exist_ok=True)
    temporary = SUBJECTS_FILE.with_suffix(".tmp")
    descriptor = os.open(temporary, os.O_WRONLY | os.O_CREAT | os.O_TRUNC, 0o600)
    with os.fdopen(descriptor, "w", encoding="ascii", newline="\n") as output:
        output.write(f"LICE_OPERATOR_SUBJECT={operator_subject}\n")
        output.write(f"LICE_VIEWER_SUBJECT={viewer_subject}\n")
    os.chmod(temporary, 0o600)
    temporary.replace(SUBJECTS_FILE)


def main() -> int:
    environment = require_environment()
    origin = environment["KEYCLOAK_ADMIN_URL"].rstrip("/")
    token = issue_token(environment)
    operator_subject = ensure_user(
        origin,
        token,
        environment["LICE_DEMO_OPERATOR_USERNAME"],
        environment["LICE_DEMO_OPERATOR_PASSWORD"],
        "Operador",
        "Global",
    )
    viewer_subject = ensure_user(
        origin,
        token,
        environment["LICE_DEMO_VIEWER_USERNAME"],
        environment["LICE_DEMO_VIEWER_PASSWORD"],
        "Leitor",
        "Sem Permissao",
    )
    publish_subjects(operator_subject, viewer_subject)
    print("Usuarios sinteticos preparados; subjects publicados para o bootstrap.")
    return 0


if __name__ == "__main__":
    try:
        raise SystemExit(main())
    except (RuntimeError, urllib.error.URLError, json.JSONDecodeError) as error:
        print(f"Falha no seed do Keycloak: {error}", file=sys.stderr)
        raise SystemExit(1) from error
