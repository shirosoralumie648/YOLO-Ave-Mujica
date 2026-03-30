from workers.zero_shot.main import summarize_batch


def summarize_video_extract(total_frames: int, ok_frames: int, failed_frames: int):
    return summarize_batch(total=total_frames, ok=ok_frames, failed=failed_frames)
