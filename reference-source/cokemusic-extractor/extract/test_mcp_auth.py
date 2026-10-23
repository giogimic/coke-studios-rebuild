#!/usr/bin/env python3
# SPDX-License-Identifier: GPL-3.0-only
import os
import unittest
from unittest import mock

import extract_castlib
import extract_room


class Response:
    def __enter__(self):
        return self

    def __exit__(self, exc_type, exc_value, traceback):
        return False

    def read(self):
        return b'{"jsonrpc":"2.0","id":1,"result":{}}'


class ExtractMcpAuthTest(unittest.TestCase):
    modules = (extract_room, extract_castlib)

    def test_call_sends_bearer_token(self):
        token = "a" * 43
        for module in self.modules:
            with self.subTest(module=module.__name__):
                with mock.patch.dict(
                    os.environ, {"DIRPLAYER_MCP_TOKEN": token}, clear=True
                ):
                    with mock.patch.object(
                        module.urllib.request,
                        "urlopen",
                        return_value=Response(),
                    ) as urlopen:
                        module.call("tools/list")

                request = urlopen.call_args.args[0]
                self.assertEqual(request.get_header("Authorization"), f"Bearer {token}")

    def test_call_rejects_missing_or_malformed_token_without_network(self):
        for module in self.modules:
            for environment in (
                {},
                {"DIRPLAYER_MCP_TOKEN": "   "},
                {"DIRPLAYER_MCP_TOKEN": "too-short"},
                {"DIRPLAYER_MCP_TOKEN": "a" * 42 + "."},
            ):
                with self.subTest(module=module.__name__, environment=environment):
                    with mock.patch.dict(os.environ, environment, clear=True):
                        with mock.patch.object(
                            module.urllib.request, "urlopen"
                        ) as urlopen:
                            with self.assertRaisesRegex(
                                RuntimeError, "DIRPLAYER_MCP_TOKEN must be"
                            ):
                                module.call("tools/list")

                    urlopen.assert_not_called()


if __name__ == "__main__":
    unittest.main()
