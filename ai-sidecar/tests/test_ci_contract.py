import unittest
from pathlib import Path


class GitLabCiContractTests(unittest.TestCase):
    def test_pipeline_runs_sidecar_tests_before_deploy(self) -> None:
        repo_root = Path(__file__).resolve().parents[2]
        ci_config = (repo_root / "ci" / "gitlab-ci.yml").read_text(encoding="utf-8")

        self.assertIn("ai-sidecar-test:", ci_config)
        self.assertIn("- job: ai-sidecar-test", ci_config)
        self.assertIn("python -B -m unittest discover ai-sidecar/tests -v", ci_config)

    def test_docker_compose_mounts_root_models_directory(self) -> None:
        repo_root = Path(__file__).resolve().parents[2]
        compose = (repo_root / "docker-compose.yml").read_text(encoding="utf-8")

        self.assertIn("./models:/models:ro", compose)
        self.assertNotIn("./ai-sidecar/models:/models:ro", compose)

    def test_compose_declares_ai_runtime_services(self) -> None:
        repo_root = Path(__file__).resolve().parents[2]
        compose = (repo_root / "docker-compose.yml").read_text(encoding="utf-8")

        self.assertIn("  inference:", compose)
        self.assertIn("dockerfile: Dockerfile.inference", compose)
        self.assertIn("  worker:", compose)
        self.assertIn("dockerfile: Dockerfile.worker", compose)
        self.assertIn("EVENT_COLLECTOR_INFERENCE_GRPC_HOST=inference", compose)

    def test_worker_and_inference_dockerfiles_match_entrypoints(self) -> None:
        repo_root = Path(__file__).resolve().parents[2]
        worker_dockerfile = (repo_root / "Dockerfile.worker").read_text(encoding="utf-8")
        inference_dockerfile = (repo_root / "Dockerfile.inference").read_text(encoding="utf-8")

        self.assertIn("apk add --no-cache ca-certificates tzdata ffmpeg", worker_dockerfile)
        self.assertIn("./cmd/worker", worker_dockerfile)
        self.assertIn('ENTRYPOINT ["/app/argus-worker"]', worker_dockerfile)
        self.assertIn("./cmd/inference", inference_dockerfile)
        self.assertIn('ENTRYPOINT ["/app/argus-inference"]', inference_dockerfile)

    def test_root_dockerignore_keeps_go_build_context_small(self) -> None:
        repo_root = Path(__file__).resolve().parents[2]
        dockerignore = (repo_root / ".dockerignore").read_text(encoding="utf-8")

        for ignored_path in (
            ".git",
            ".gocache",
            ".go-build-cache",
            ".golangci-cache",
            ".bin",
            "ai-sidecar",
            "models",
            "*.onnx",
            "*.mp4",
            "gosec.sarif",
        ):
            self.assertIn(ignored_path, dockerignore)


if __name__ == "__main__":
    unittest.main()
