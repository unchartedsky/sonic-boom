#!/bin/bash
set -e

export ROOT_DIR="$(git rev-parse --show-toplevel)"

getshell() {
  echo $(basename $SHELL)
}

install_brew() {
  type brew &>/dev/null || /usr/bin/ruby -e "$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/master/install)"

  brew bundle
}

# See https://pre-commit.com/#install
install_precommit() {
  pre-commit install
  pre-commit autoupdate
}

sudo -v

install_brew

install_precommit
