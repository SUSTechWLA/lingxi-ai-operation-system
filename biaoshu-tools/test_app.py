import json
import sys
import tempfile
import unittest
from pathlib import Path
from unittest.mock import patch

sys.path.insert(0, str(Path(__file__).resolve().parent))

import app


class ParseBidFilesTests(unittest.TestCase):
    def test_parse_bid_files_does_not_call_llm_and_returns_raw_text(self):
        with tempfile.TemporaryDirectory() as temp_dir:
            bid_file = Path(temp_dir) / "test_bid.txt"
            bid_file.write_text("招标文件正文", encoding="utf-8")

            fake_script = {
                "success": True,
                "data": {
                    "stdout": "招标文件正文",
                    "stderr": "",
                },
            }

            client = app.app.test_client()
            with patch.object(app, "OUTPUT_DIR", Path(temp_dir) / "output"), \
                    patch.object(app, "run_script", return_value=fake_script), \
                    patch("urllib.request.urlopen") as urlopen:
                response = client.post("/tools/parse_bid_files", json={
                    "params": {
                        "file_path": str(bid_file),
                        "output_dir": "demo-project",
                    }
                })

            self.assertEqual(response.status_code, 200)
            payload = json.loads(response.data.decode("utf-8"))
            self.assertTrue(payload["success"])
            data = payload["data"]
            self.assertEqual(data["stdout"], "招标文件正文")
            self.assertTrue(data["report_path"].endswith("00_招标文件解析报告.md"))
            self.assertTrue(data["raw_text_path"].endswith("00_招标文件原文解析.md"))
            self.assertEqual(data["source_file"], str(bid_file))
            self.assertFalse(Path(data["report_path"]).exists())
            self.assertTrue(Path(data["raw_text_path"]).exists())
            urlopen.assert_not_called()


if __name__ == "__main__":
    unittest.main()
