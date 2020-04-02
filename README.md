# Bodman

Boadman is a minimum feature set of docker and podman. The original intuition is runing container under systemd more similar to a normal binary.

Inspired by [bocker](https://github.com/p8952/bocker/blob/master/bocker), [podman](https://podman.io) and [rkt](https://github.com/rkt/rkt). Current implementation is a POC, further rewriting in other reasonable language (Go?) is planned.

A lot of features are still missing. If you find your docker image broken in bodman, just file a issue.

## Dependencies

- ostree
- python3.7 or higer
- skopeo

## Usage

You must pull you image manully before you use it (Maybe change in future).
```bash
bodman pull debian:testing
bodman run -t debian:testing /bin/bash
```

Current support `run` arguments:
```bash
usage: bodman run [--env ENV] [--hostname HOSTNAME] [--tty] [--user USER]
                  [--volume VOLUME] [--workdir WORKDIR]
                  image ...
```

## Roadmap

- Rewrite in Go
- Fetch local image from docker/podman
- Maybe: CNI Plugin support
- Maybe: OverlayFS and fuse-overlay

### Goal

- Only support minimum feature, which removes a lot of complexity.
- Make it using like `docker` in most case.

### Non Goal

- Build Image: It's only a complementary to current container ecosystem. You should still build your image using `docker` or `podman`.
- Daemon: Support running container as a daemon means we need a `containerd`. Like `rkt`, you can do it youself using systemd or just using `podman` or `docker`.