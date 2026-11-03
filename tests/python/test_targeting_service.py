from tests.python.support import FakeConnection, load_service


def test_targeting_match_evaluates_all_enabled_rules(monkeypatch):
    module = load_service("services/targeting-service/app.py", "targeting_service_test")
    connection = FakeConnection(
        rows=[
            ("country", "equals", "BR"),
            ("plan", "in", "pro,team"),
        ]
    )
    monkeypatch.setattr(module, "connection", lambda: connection)

    response = module.app.test_client().post(
        "/targeting/match/checkout",
        json={"attributes": {"country": "BR", "plan": "pro"}},
    )

    assert response.status_code == 200
    assert response.get_json() == {
        "flag_name": "checkout",
        "matches": True,
        "rules_evaluated": 2,
    }


def test_targeting_match_fails_when_attribute_does_not_match(monkeypatch):
    module = load_service("services/targeting-service/app.py", "targeting_negative_test")
    connection = FakeConnection(rows=[("country", "equals", "BR")])
    monkeypatch.setattr(module, "connection", lambda: connection)

    response = module.app.test_client().post(
        "/targeting/match/checkout",
        json={"attributes": {"country": "US"}},
    )

    assert response.status_code == 200
    assert response.get_json()["matches"] is False
