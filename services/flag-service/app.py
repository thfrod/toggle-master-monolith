import os
import time
from datetime import datetime, timezone

import psycopg2
from flask import Flask, jsonify, request

app = Flask(__name__)


def database_url() -> str:
    return os.getenv(
        "FLAGS_DATABASE_URL",
        "postgres://user:password@postgres-flags:5432/flags?sslmode=disable",
    )


def connection():
    return psycopg2.connect(database_url())


def init_db() -> None:
    last_error = None
    for _ in range(30):
        try:
            with connection() as conn, conn.cursor() as cur:
                cur.execute(
                    """
                    CREATE TABLE IF NOT EXISTS flags (
                        id SERIAL PRIMARY KEY,
                        name VARCHAR(100) UNIQUE NOT NULL,
                        is_enabled BOOLEAN NOT NULL DEFAULT FALSE,
                        description TEXT,
                        created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
                        updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
                    )
                    """
                )
            return
        except psycopg2.Error as exc:
            last_error = exc
            time.sleep(2)
    raise RuntimeError(f"could not initialize flags database: {last_error}")


def flag_json(row):
    return {
        "id": row[0],
        "name": row[1],
        "is_enabled": row[2],
        "description": row[3],
        "created_at": row[4].isoformat() if row[4] else None,
        "updated_at": row[5].isoformat() if row[5] else None,
    }


@app.get("/health")
def health():
    try:
        with connection() as conn:
            with conn.cursor() as cur:
                cur.execute("SELECT 1")
        return jsonify(status="ok")
    except psycopg2.Error:
        return jsonify(status="degraded", error="database unavailable"), 503


@app.route("/flags", methods=["GET", "POST"])
def flags():
    if request.method == "GET":
        with connection() as conn, conn.cursor() as cur:
            cur.execute(
                "SELECT id, name, is_enabled, description, created_at, updated_at FROM flags ORDER BY name"
            )
            return jsonify([flag_json(row) for row in cur.fetchall()])

    payload = request.get_json(silent=True) or {}
    name = str(payload.get("name", "")).strip()
    if not name:
        return jsonify(error="name is required"), 400
    try:
        with connection() as conn, conn.cursor() as cur:
            cur.execute(
                """
                INSERT INTO flags (name, is_enabled, description)
                VALUES (%s, %s, %s)
                RETURNING id, name, is_enabled, description, created_at, updated_at
                """,
                (name, bool(payload.get("is_enabled", False)), payload.get("description")),
            )
            return jsonify(flag_json(cur.fetchone())), 201
    except psycopg2.errors.UniqueViolation:
        return jsonify(error="flag already exists"), 409


@app.route("/flags/<string:name>", methods=["GET", "PUT", "DELETE"])
def flag(name: str):
    if request.method == "GET":
        with connection() as conn, conn.cursor() as cur:
            cur.execute(
                "SELECT id, name, is_enabled, description, created_at, updated_at FROM flags WHERE name = %s",
                (name,),
            )
            row = cur.fetchone()
            return jsonify(flag_json(row)) if row else (jsonify(error="flag not found"), 404)

    if request.method == "DELETE":
        with connection() as conn, conn.cursor() as cur:
            cur.execute("DELETE FROM flags WHERE name = %s", (name,))
            if cur.rowcount == 0:
                return jsonify(error="flag not found"), 404
        return jsonify(message="flag deleted")

    payload = request.get_json(silent=True) or {}
    if not isinstance(payload.get("is_enabled"), bool):
        return jsonify(error="is_enabled must be boolean"), 400
    with connection() as conn, conn.cursor() as cur:
        cur.execute(
            """
            UPDATE flags
            SET is_enabled = %s, description = COALESCE(%s, description), updated_at = CURRENT_TIMESTAMP
            WHERE name = %s
            RETURNING id, name, is_enabled, description, created_at, updated_at
            """,
            (payload["is_enabled"], payload.get("description"), name),
        )
        row = cur.fetchone()
        return jsonify(flag_json(row)) if row else (jsonify(error="flag not found"), 404)


if os.getenv("SKIP_DB_INIT") != "1":
    init_db()
