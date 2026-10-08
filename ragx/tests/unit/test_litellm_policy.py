from thomas_ragx.platform.litellm_policy import LOCAL_COST_MAP_ENV, apply_litellm_policy


def test_policy_pins_the_bundled_model_map() -> None:
    environ: dict[str, str] = {}
    apply_litellm_policy(environ)
    assert environ[LOCAL_COST_MAP_ENV] == "True"
