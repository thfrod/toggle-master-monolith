from tests.python.support import load_service


class FakeTable:
    def __init__(self):
        self.item = None

    def put_item(self, Item):
        self.item = Item


def test_save_event_normalizes_analytics_item():
    module = load_service("services/analytics-service/app.py", "analytics_service_test")
    table = FakeTable()

    module.save_event(
        table,
        {
            "event_id": "event-1",
            "flag_name": "checkout",
            "subject_id": "user-1",
            "enabled": True,
            "attributes": {"country": "BR"},
        },
    )

    assert table.item["event_id"] == "event-1"
    assert table.item["flag_name"] == "checkout"
    assert table.item["enabled"] is True
    assert '"country":"BR"' in table.item["attributes_json"]
