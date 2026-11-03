import os
import time

import psycopg2
from flask import Flask, jsonify, request

app = Flask(__name__)


def connection():
    return psycopg2.connect(
        os.getenv(
            "TARGETING_DATABASE_URL",
            "postgres://user:password@postgres-flags:5432/flags?sslmode=disable",
        )
    )


def init_db():
    last_error = None
    for _ in range(30):
        try:
            with connection() as conn, conn.cursor() as cur:
                cur.execute(
                    """
                    CREATE TABLE IF NOT EXISTS targeting_rules (
                        id SERIAL PRIMARY KEY,
                        flag_name VARCHAR(100) NOT NULL,
                        attribute VARCHAR(100) NOT NULL,
                        operator VARCHAR(20) NOT NULL DEFAULT 'equals',
                        expected_value TEXT NOT NULL,
                        enabled BOOLEAN NOT NULL DEFAULT TRUE,
                        created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
                    )
                    """
                )
            return
        except psycopg2.Error as exc:
            last_error = exc
            time.sleep(2)
    raise RuntimeError(f"could not initialize targeting database: {last_error}")


def rule_json(row):
    return {
        "id": row[0],
        "flag_name": row[1],
        "attribute": row[2],
        "operator": row[3],
        "expected_value": row[4],
        "enabled": row[5],
        "created_at": row[6].isoformat() if row[6] else None,
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


@app.route("/targeting/rules", methods=["GET", "POST"])
def rules():
    if request.method == "GET":
        flag_name = request.args.get("flag_name")
        query = "SELECT id, flag_name, attribute, operator, expected_value, enabled, created_at FROM targeting_rules"
        params = []
        if flag_name:
            query += " WHERE flag_name = %s"
            params.append(flag_name)
        query += " ORDER BY id"
        with connection() as conn, conn.cursor() as cur:
            cur.execute(query, params)
            return jsonify([rule_json(row) for row in cur.fetchall()])

    payload = request.get_json(silent=True) or {}
    required = ("flag_name", "attribute", "expected_value")
    if any(not str(payload.get(field, "")).strip() for field in required):
        return jsonify(error="flag_name, attribute and expected_value are required"), 400
    with connection() as conn, conn.cursor() as cur:
        cur.execute(
            """
            INSERT INTO targeting_rules (flag_name, attribute, operator, expected_value, enabled)
            VALUES (%s, %s, %s, %s, %s)
            RETURNING id, flag_name, attribute, operator, expected_value, enabled, created_at
            """,
            (
                payload["flag_name"],
                payload["attribute"],
                payload.get("operator", "equals"),
                str(payload["expected_value"]),
                bool(payload.get("enabled", True)),
            ),
        )
        return jsonify(rule_json(cur.fetchone())), 201


@app.post("/targeting/match/<string:flag_name>")
def match(flag_name: str):
    payload = request.get_json(silent=True) or {}
    attributes = payload.get("attributes", {})
    with connection() as conn, conn.cursor() as cur:
        cur.execute(
            """
            SELECT attribute, operator, expected_value
            FROM targeting_rules
            WHERE flag_name = %s AND enabled = TRUE
            ORDER BY id
            """,
            (flag_name,),
        )
        rows = cur.fetchall()

    def matches(attribute, operator, expected):
        actual = attributes.get(attribute)
        if actual is None:
            return False
        expected_values = [part.strip() for part in expected.split(",")]
        if operator == "in":
            return str(actual) in expected_values
        if operator == "contains":
            return expected in str(actual)
        return str(actual) == expected

    result = all(matches(*row) for row in rows) if rows else True
    return jsonify({"flag_name": flag_name, "matches": result, "rules_evaluated": len(rows)})


if os.getenv("SKIP_DB_INIT") != "1":
    init_db()
