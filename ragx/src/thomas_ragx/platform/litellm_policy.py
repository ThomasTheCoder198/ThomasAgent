from collections.abc import MutableMapping
from typing import Final

LOCAL_COST_MAP_ENV: Final = "LITELLM_LOCAL_MODEL_COST_MAP"


def apply_litellm_policy(environ: MutableMapping[str, str]) -> None:
    """Pin LiteLLM's bundled model map, avoiding a download during import."""
    environ[LOCAL_COST_MAP_ENV] = "True"
