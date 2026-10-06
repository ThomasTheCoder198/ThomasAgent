FROM busybox:1.37.0-musl AS probe
FROM victoriametrics/victoria-logs:v1.53.0

COPY --from=probe /bin/busybox /bin/busybox
