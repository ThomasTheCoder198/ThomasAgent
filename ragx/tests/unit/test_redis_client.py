from thomas_ragx.platform.config import Config
from thomas_ragx.resources import REDIS_PROTOCOL, create_redis


def test_redis_client_takes_its_socket_timeout_and_protocol_from_config() -> None:
    config = Config(minio_access_key="a", minio_secret_key="s", redis_socket_timeout_seconds=7.5)

    client = create_redis(config)

    kwargs = client.connection_pool.connection_kwargs
    assert kwargs["socket_timeout"] == 7.5
    assert kwargs["protocol"] == REDIS_PROTOCOL
    assert kwargs["decode_responses"] is True
