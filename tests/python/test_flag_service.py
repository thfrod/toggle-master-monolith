from datetime import datetime, timezone

from tests.python.support import FakeConnection, load_service


def test_list_flags_returns_database_rows(monkeypatch):
    module = load_service("services/flag-service/app.py", "flag_service_test")
    created = datetime(2026, 1, 1, tzinfo=timezone.utc)
    connection = FakeConnection(
        rows=[(1, "new-checkout", True, "new flow", created, created)]
    )
    monkeypatch.setattr(module, "connection", lambda: connection)

    response = module.app.test_client().get("/flags")

    assert response.status_code == 200
    assert response.get_json()[0]["name"] == "new-checkout"
    assert response.get_json()[0]["is_enabled"] is True


def test_create_flag_validates_name():
    module = load_service("services/flag-service/app.py", "flag_service_validation_test")

    response = module.app.test_client().post("/flags", json={"is_enabled": True})

    assert response.status_code == 400
    assert response.get_json()["error"] == "name is required"
