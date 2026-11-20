FROM archlinux:latest

# jq is used by the smoke test to assert the --json contract. tree is
# intentionally NOT installed: the smoke test installs it through harpy.
RUN pacman -Syu --noconfirm \
    && pacman -S --noconfirm --needed ca-certificates git go jq less \
    && pacman -Scc --noconfirm

WORKDIR /src
