"""MCP discovery -> LangChain, with small application-specific execution policies."""
import copy
import json
import re
from jsonschema import validate, ValidationError
from langchain_core.tools import StructuredTool
from chat.models import AgentError

def discover_tools(gateway, token, turn, update_task, whoami="whoami"):
    tools = []
    for spec in gateway.discover(token):
        name = spec["name"]
        if name == whoami:
            continue  # Identity is mandatory preparation, never an LLM choice.
        schema = copy.deepcopy(spec["schema"])
        properties = schema.setdefault("properties", {})
        if name == "create_time_off_request":
            properties.pop("idempotency_key", None)
            schema["required"] = [x for x in schema.get("required", []) if x != "idempotency_key"]
        if name == "update_task_status" and "task_id" in properties:
            # Retain task-name resolution and the existing progress guard.
            properties.pop("task_id")
            properties["task"] = {"type": "string", "description": "ID или часть названия задачи"}
            schema["required"] = ["task" if x == "task_id" else x for x in schema.get("required", [])]
        def make_execute(_spec, _schema):
            def execute(**args):
                try:
                    validate(args, _schema)
                except ValidationError:
                    return json.dumps({"error": "Неверные или отсутствующие поля. Уточни их у пользователя."}, ensure_ascii=False)
                if _spec["name"] == "update_task_status":
                    return update_task.invoke(args)
                if _spec["name"] == "create_time_off_request":
                    if not str(args.get("reason", "")).strip():
                        return json.dumps({"error": "Сначала спроси у пользователя причину отгула. Не придумывай её."}, ensure_ascii=False)
                    # Require verbatim user evidence, never an invented placeholder.
                    normalize = lambda text: " ".join(re.findall(r"\w+", str(text).casefold()))
                    reason = normalize(args["reason"])
                    user_messages = [m for m in getattr(turn, "history", []) if m.role == "user"]
                    current = getattr(turn, "user_message", None)
                    if current is not None:
                        user_messages.append(current)
                    if not reason or not any(reason in normalize(m.content) for m in user_messages):
                        return json.dumps({"error": "Причина не найдена в словах пользователя. Спроси причину и передай её дословно, без выдумок."}, ensure_ascii=False)
                    args["idempotency_key"] = "time-off:" + turn.run_id
                result = gateway.call(_spec["name"], args, token, url=_spec["url"])
                return json.dumps(result, ensure_ascii=False)
            return execute
        tools.append(StructuredTool(name=name, description=spec["description"],
                                    args_schema=schema, func=make_execute(spec, schema)))
    return tools
