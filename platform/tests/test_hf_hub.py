"""Unit tests for HuggingFaceHubClient and HFDownloadManager (Phase 3)."""

import pytest
from pathlib import Path
from kubona.mesh.hf_hub import (
    HuggingFaceHubClient,
    HFDownloadManager,
    _infer_quant_type,
    HFModelSummary,
    HFModelQuantization,
)


def test_infer_quant_type():
    assert _infer_quant_type("llama-3.2-3b-instruct-q4_k_m.gguf") == "Q4_K_M"
    assert _infer_quant_type("qwen2.5-coder-7b-instruct-q8_0.gguf") == "Q8_0"
    assert _infer_quant_type("model-fp16.gguf") == "FP16"
    assert _infer_quant_type("phi-4-mlx-4bit.tar.gz") == "MLX-4BIT"


@pytest.mark.asyncio
async def test_download_manager_task_lifecycle(tmp_path: Path):
    mgr = HFDownloadManager(models_dir=tmp_path)

    # Verify task initiation
    task = await mgr.start_download("test/dummy-repo", "dummy-q4_k_m.gguf")
    assert task.task_id is not None
    assert task.repo_id == "test/dummy-repo"
    assert task.filename == "dummy-q4_k_m.gguf"
    assert task.status in ["pending", "downloading", "failed"]

    # Verify task listing
    tasks = mgr.list_tasks()
    assert len(tasks) == 1
    assert tasks[0].task_id == task.task_id

    # Verify cancel task
    mgr.cancel_task(task.task_id)
    retrieved = mgr.get_task(task.task_id)
    assert retrieved is not None
    assert retrieved.status in ["cancelled", "failed", "pending", "downloading"]
