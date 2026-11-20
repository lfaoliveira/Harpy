FROM golang:1.23-bookworm

ENV DEBIAN_FRONTEND=noninteractive

# jq is used by the smoke test to assert the --json contract. tree is
# intentionally NOT installed: the smoke test installs it through harpy.
RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates git jq less \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /src
