from thomas_ragx.retrieval.segmenter import IdentitySegmenter, UndertheseaSegmenter, create_segmenter


def test_underthesea_joins_vietnamese_compounds() -> None:
    assert "hợp_đồng" in UndertheseaSegmenter().segment("Hợp đồng lao động").lower()


def test_identity_and_factory() -> None:
    assert IdentitySegmenter().segment("a b") == "a b"
    assert isinstance(create_segmenter("none"), IdentitySegmenter)
    assert isinstance(create_segmenter("underthesea"), UndertheseaSegmenter)
