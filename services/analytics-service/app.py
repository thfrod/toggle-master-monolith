import json
import logging
import os
import threading
import time
import uuid
from datetime import datetime, timezone

import boto3
import redis
from flask import Flask, jsonify

logging.basicConfig(level=os.getenv("LOG_LEVEL", "INFO"))
logger = logging.getLogger("analytics-service")
app = Flask(__name__)

TABLE_NAME = os.getenv("ANALYTICS_TABLE", "togglemaster-analytics")
EVENT_DRIVER = os.getenv("EVENT_BUS_DRIVER", "redis")
QUEUE_KEY = os.getenv("REDIS_QUEUE_KEY", "analytics-events")
AWS_REGION = os.getenv("AWS_REGION", "us-east-1")


def dynamodb_resource():
    endpoint = os.getenv("DYNAMODB_ENDPOINT_URL")
    options = {"region_name": AWS_REGION}
    if endpoint:
        options.update(
            {
                "endpoint_url": endpoint,
                "aws_access_key_id": os.getenv("AWS_ACCESS_KEY_ID", "dummy"),
                "aws_secret_access_key": os.getenv("AWS_SECRET_ACCESS_KEY", "dummy"),
            }
        )
    return boto3.resource("dynamodb", **options)


def ensure_table():
    resource = dynamodb_resource()
    try:
        table = resource.Table(TABLE_NAME)
        table.load()
        return table
    except resource.meta.client.exceptions.ResourceNotFoundException:
        logger.info("creating DynamoDB table %s", TABLE_NAME)
        table = resource.create_table(
            TableName=TABLE_NAME,
            KeySchema=[{"AttributeName": "event_id", "KeyType": "HASH"}],
            AttributeDefinitions=[{"AttributeName": "event_id", "AttributeType": "S"}],
            BillingMode="PAY_PER_REQUEST",
        )
        table.wait_until_exists()
        return table


def save_event(table, event):
    event_id = str(event.get("event_id") or uuid.uuid4())
    item = {
        "event_id": event_id,
        "flag_name": str(event.get("flag_name", "unknown")),
        "subject_id": str(event.get("subject_id", "anonymous")),
        "enabled": bool(event.get("enabled", False)),
        "cached": bool(event.get("cached", False)),
        "attributes_json": json.dumps(event.get("attributes", {}), separators=(",", ":")),
        "evaluated_at": str(event.get("evaluated_at", datetime.now(timezone.utc).isoformat())),
        "received_at": datetime.now(timezone.utc).isoformat(),
    }
    table.put_item(Item=item)


def receive_from_redis():
    client = redis.Redis.from_url(
        "redis://" + os.getenv("REDIS_ADDR", "redis:6379"),
        password=os.getenv("REDIS_PASSWORD") or None,
        decode_responses=True,
    )
    while True:
        try:
            message = client.brpop(QUEUE_KEY, timeout=5)
            if message:
                _, payload = message
                yield payload
        except Exception as exc:
            logger.warning("redis consumer error: %s", exc)
            time.sleep(2)


def receive_from_sqs():
    client = boto3.client(
        "sqs",
        region_name=AWS_REGION,
        aws_access_key_id=os.getenv("AWS_ACCESS_KEY_ID"),
        aws_secret_access_key=os.getenv("AWS_SECRET_ACCESS_KEY"),
    )
    queue_url = os.environ["SQS_QUEUE_URL"]
    while True:
        response = client.receive_message(
            QueueUrl=queue_url,
            MaxNumberOfMessages=10,
            WaitTimeSeconds=20,
            VisibilityTimeout=60,
        )
        for message in response.get("Messages", []):
            yield message["Body"]
            client.delete_message(QueueUrl=queue_url, ReceiptHandle=message["ReceiptHandle"])


def worker():
    while True:
        try:
            table = ensure_table()
            receiver = receive_from_redis() if EVENT_DRIVER == "redis" else receive_from_sqs()
            for payload in receiver:
                try:
                    save_event(table, json.loads(payload))
                except Exception:
                    logger.exception("could not persist analytics event")
        except Exception:
            logger.exception("analytics worker failed; retrying")
            time.sleep(5)


@app.get("/health")
def health():
    try:
        table = dynamodb_resource().Table(TABLE_NAME)
        table.load()
        return jsonify(status="ok")
    except Exception as exc:
        return jsonify(status="degraded", error=str(exc)), 503


@app.get("/analytics/events")
def events():
    try:
        table = dynamodb_resource().Table(TABLE_NAME)
        response = table.scan(Limit=50)
        return jsonify(response.get("Items", []))
    except Exception as exc:
        return jsonify(error=str(exc)), 503


if os.getenv("SKIP_WORKER") != "1":
    ensure_table()
    threading.Thread(target=worker, name="analytics-consumer", daemon=True).start()
