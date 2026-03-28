def emit_item_error(job_id: int, item_id: int, message: str, detail: dict):
    payload = {
        "job_id": job_id,
        "item_id": item_id,
        "event_level": "error",
        "event_type": "item_failed",
        "message": message,
        "detail_json": detail,
    }
    return payload
