import unittest
from pathlib import Path


class GitLabCiContractTests(unittest.TestCase):
    def test_pipeline_runs_sidecar_tests_before_deploy(self) -> None:
        repo_root = Path(__file__).resolve().parents[2]
        ci_config = (repo_root / "ci" / "gitlab-ci.yml").read_text(encoding="utf-8")

        self.assertIn("ai-sidecar-test:", ci_config)
        self.assertIn("- job: ai-sidecar-test", ci_config)
        self.assertIn("python -B -m unittest discover ai-sidecar/tests -v", ci_config)


if __name__ == "__main__":
    unittest.main()
