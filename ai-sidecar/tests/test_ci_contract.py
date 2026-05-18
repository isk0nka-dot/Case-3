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


if __name__ == "__main__":
    unittest.main()
