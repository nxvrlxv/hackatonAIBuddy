import unittest
from unittest.mock import patch
from dataclasses import replace
from test_chat import make_settings
from clients.llm import create_model
from memory.context import merge_system_messages
from langchain_core.messages import SystemMessage, HumanMessage

class LLMTests(unittest.TestCase):
    def test_gigachat_factory(self):
        s=replace(make_settings(),llm_provider="gigachat",model="GigaChat-2",gigachat_credentials="secret")
        self.assertNotIn("secret",repr(s))
        with patch("langchain_gigachat.GigaChat") as constructor:
            create_model(s)
        kw=constructor.call_args.kwargs
        self.assertEqual(kw["credentials"],"secret")
        self.assertNotIn("access_token",kw)
        self.assertTrue(kw["verify_ssl_certs"])
        self.assertTrue(kw["streaming"])

    def test_system_profile_and_summary_are_preserved(self):
        messages=merge_system_messages([SystemMessage(content="rules"),SystemMessage(content="profile"),
            SystemMessage(content="summary"),HumanMessage(content="hello")])
        self.assertEqual(len(messages),2)
        self.assertEqual(messages[0].content,"rules\n\nprofile\n\nsummary")
        self.assertEqual(messages[1].content,"hello")
