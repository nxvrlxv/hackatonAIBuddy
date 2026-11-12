"""Контракт данных агента и Go. Здесь нет ORM: БД принадлежит Go."""
from typing import Literal
from pydantic import BaseModel, Field, ConfigDict
from langchain_core.messages import AIMessage, HumanMessage, ToolMessage

class AgentError(Exception):
    def __init__(self, code: str, status: int = 502):
        self.code, self.status = code, status
        super().__init__(code)

class User(BaseModel):
    # Неизвестные поля whoami (в том числе token) не попадут в контекст.
    model_config = ConfigDict(extra="ignore")
    id: int = Field(gt=0)
    full_name: str = Field(min_length=1, max_length=200)
    team_id: int | None = None
    team: str | None = Field(default=None, max_length=200)
    role: str = Field(max_length=80)
    mentor: str | None = Field(default=None, max_length=200)

class Summary(BaseModel):
    text: str = ""
    through_sequence: int = 0
    version: int = 0

class MessagePayload(BaseModel):
    # Дополнительные поля остаются в JSON, не требуют новых SQL-колонок.
    model_config = ConfigDict(extra="allow")
    content: str | list = ""
    tool_calls: list[dict] = Field(default_factory=list)
    tool_call_id: str | None = None

def storage_message(data):
    """Упаковывает внутренние аргументы в единый JSON-контракт Go."""
    data = dict(data)
    if "payload" not in data:
        data["payload"] = {key: data.pop(key) for key in
                           ("content", "tool_calls", "tool_call_id") if key in data}
    data["payload"] = MessagePayload.model_validate(data["payload"]).model_dump(exclude_none=True)
    return data

class Message(BaseModel):
    model_config = ConfigDict(extra="forbid")
    id: int
    sequence_number: int
    run_id: str
    role: Literal["user", "assistant", "tool"]
    payload: MessagePayload
    status: Literal["completed", "interrupted", "failed"] = "completed"

    # Удобный доступ внутри Python. В HTTP/БД поля остаются внутри payload.
    @property
    def content(self):
        return self.payload.content

    @property
    def tool_calls(self):
        return self.payload.tool_calls

    @property
    def tool_call_id(self):
        return self.payload.tool_call_id

    def as_langchain(self):
        if self.role == "user":
            return HumanMessage(content=self.content)
        if self.role == "tool":
            if not self.tool_call_id:
                raise AgentError("invalid_tool_history")
            return ToolMessage(content=self.content, tool_call_id=self.tool_call_id)
        return AIMessage(content=self.content, tool_calls=self.tool_calls)

class Turn(BaseModel):
    run_id: str
    conversation_id: int
    user_id: int
    summary: Summary = Field(default_factory=Summary)
    history: list[Message]
    user_message: Message

class MemoryHit(BaseModel):
    user_id: int
    conversation_id: int
    start_sequence: int
    end_sequence: int
    content: str
    created_at: str | None = None
