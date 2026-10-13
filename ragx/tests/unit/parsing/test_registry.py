from thomas_ragx.kb.upload import SUPPORTED_EXTENSIONS
from thomas_ragx.parsing.registry import IMAGE_EXTENSIONS, local_registry


def test_every_supported_non_image_extension_has_a_local_parser() -> None:
    registry = local_registry()
    for extension in SUPPORTED_EXTENSIONS:
        parser = registry.for_extension(extension)
        assert (parser is None) == (extension in IMAGE_EXTENSIONS), extension
