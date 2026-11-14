"""Детерминированный фильтр явных запросов кода и внутреннего API до LLM."""
import re
import unicodedata

REFUSAL = ("Я помогаю с рабочими задачами и онбордингом. "
           "Я не пишу программы и не показываю внутренние схемы или вызовы инструментов. "
           "Могу объяснить порядок действий или помочь оформить заявку.")

def refusal_for(content):
    if isinstance(content, list):
        content = " ".join(b.get("text", "") for b in content if isinstance(b, dict))
    text = unicodedata.normalize("NFKC", str(content)).casefold()
    text = re.sub(r"[\u200b-\u200f\ufeff]", "", text)
    request = re.search(r"напиш|написа|покаж|показа|сгенер|сдела|созда|дай|вывед|вывест|"
                        r"пришл|привед|пример|постро|нарис|write|generate|show|give|create", text)
    code = re.search(r"\bкод(?:а|ом)?\b|скрипт|программ[ау]|бот[а]?\b|"
                     r"python|пайтон|питон|javascript|typescript|matplotlib|исходник|псевдокод|"
                     r"\bcode\b|\bscript\b", text)
    internal = re.search(r"тулз|тул[ауы]?\b|инструмент|tool|function.?call|"
                         r"create_time_off_request|get_request_types|list_my_requests|"
                         r"системн.{0,20}промпт|внутренн.{0,20}(?:контекст|профил)", text)
    expose = re.search(r"схем|синтаксис|сигнатур|аргумент|параметр|пример|псевдо|json|"
                       r"как.{0,30}вызва|вызов|schema|signature|prompt", text)
    profile_dump = re.search(r"json|схем", text) and re.search(r"обо мне|профил|контекст|мои данные", text)
    if (request and code) or (internal and expose) or profile_dump:
        return REFUSAL
    return None
