import json
import re
from importlib.resources import files

from thomas_ragx.platform.error_codes_gen import ErrorCode
from thomas_ragx.platform.errors import Lang
from thomas_ragx.ui.i18n import FIELD_ERRORS, error_message, error_text, field_error_key

# The lookbehind keeps `form.get('kind')` from reading as the translator call `t('kind')`.
TEMPLATE_KEY = re.compile(r"""(?<![\w.])t\(['"]([a-z_.]+)['"]\)""")


def _messages(lang: str) -> dict[str, str]:
    raw = files("thomas_ragx.ui").joinpath("messages", f"{lang}.json").read_text(encoding="utf-8")
    return dict(json.loads(raw))


def test_vietnamese_and_english_have_the_same_keys() -> None:
    assert _messages("vi").keys() == _messages("en").keys()


def test_every_key_used_by_a_template_exists() -> None:
    used: set[str] = set()
    for template in files("thomas_ragx.ui").joinpath("templates").iterdir():
        used |= set(TEMPLATE_KEY.findall(template.read_text(encoding="utf-8")))
    assert used <= _messages("vi").keys()


def test_every_field_error_the_ui_can_carry_has_a_message_in_both_languages() -> None:
    for lang in ("vi", "en"):
        messages = _messages(lang)
        for field, reason in FIELD_ERRORS:
            assert field_error_key(field, reason) in messages, (lang, field, reason)


def test_a_crafted_field_or_reason_falls_back_to_the_catalog_message() -> None:
    code = ErrorCode.VALIDATION_FAILED.value
    generic = error_text(code, Lang.EN)
    assert error_message(code, ("../evil", "REQUIRED"), Lang.EN) == generic
    assert error_message(code, ("apiKey", "SURPRISE"), Lang.EN) == generic
    assert error_message(code, None, Lang.EN) == generic


def test_a_field_error_only_replaces_the_validation_message() -> None:
    other = ErrorCode.LLM_PROVIDER_REJECTED.value
    assert error_message(other, ("apiKey", "REQUIRED"), Lang.EN) == error_text(other, Lang.EN)
    assert error_message(ErrorCode.VALIDATION_FAILED.value, ("apiKey", "REQUIRED"), Lang.EN) != error_text(
        ErrorCode.VALIDATION_FAILED.value, Lang.EN
    )
