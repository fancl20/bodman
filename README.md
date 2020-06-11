# Bodman

Boadman implements minimum feature set of docker and podman. The original intuition is runing container under systemd more similar to a normal binary.

Inspired by [bocker](https://github.com/p8952/bocker/blob/master/bocker), [podman](https://podman.io) and [rkt](https://github.com/rkt/rkt).

It's only designed for running trusty image. If you find your docker image broken in bodman, just file a issue.

## Dependencies

- ostree

## Usage

You must pull you image manully before you use it (Maybe change in future).
```bash
bodman pull debian:testing
bodman run debian:testing /bin/bash
```

Current support `run` arguments:
```bash
NAME:
   bodman run -

USAGE:
   bodman run [command options] [arguments...]

OPTIONS:
   --help                      (default: false)
   --env value, -e value
   --hostname value, -h value
   --systemd-activation        (default: false)
   --user value, -u value
   --volume value, -v value
   --workdir value, -w value
```

## Roadmap

- Fetch local image from docker/podman
- Maybe: CNI Plugin support
- Maybe: OverlayFS and fuse-overlay

### Goal

- Only support minimum feature, which removes a lot of complexities.
- Make it using like `docker` in most cases.

### Non Goal
- PID Namespace: It only works when we fork a new process. That means we need a "supervisor" for the forked process. To simplify the design, just remove PID Namespace support because isolation is not the target.
- Isolation & Security: It's only designed for running trusty image. And removing features like cpu/memory limitation and capability control can eliminates most part of code.
- Build Image: It's only a complementary to current container ecosystem. You should still build your image using `docker` or `podman`.
- Daemon: Support running container as a daemon means we need a `containerd`. Like `rkt`, you can do it youself using systemd or just using `podman` or `docker`.