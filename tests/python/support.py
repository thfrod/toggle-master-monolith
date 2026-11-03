import importlib.util
import os
from pathlib import Path


ROOT = Path(__file__).parents[2]


def load_service(relative_path, module_name):
    """Load a Flask service without connecting to its external dependencies."""
    os.environ["SKIP_DB_INIT"] = "1"
    os.environ["SKIP_WORKER"] = "1"
    path = ROOT / relative_path
    spec = importlib.util.spec_from_file_location(module_name, path)
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


class FakeCursor:
    def __init__(self, rows=(), row=None):
        self.rows = list(rows)
        self.row = row
        self.executed = []

    def __enter__(self):
        return self

    def __exit__(self, *_):
        return False

    def execute(self, query, params=None):
        self.executed.append((query, params))

    def fetchall(self):
        return self.rows

    def fetchone(self):
        if self.row is not None:
            return self.row
        return self.rows[0] if self.rows else None


class FakeConnection:
    def __init__(self, rows=(), row=None):
        self.cursor_instance = FakeCursor(rows=rows, row=row)

    def __enter__(self):
        return self

    def __exit__(self, *_):
        return False

    def cursor(self):
        return self.cursor_instance
