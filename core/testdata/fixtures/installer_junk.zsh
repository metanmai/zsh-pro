export EDITOR=nvim

# >>> conda initialize >>>
__conda_setup="$('/opt/conda/bin/conda' shell.zsh hook 2> /dev/null)"
eval "$__conda_setup"
# <<< conda initialize <<<

export NVM_DIR="$HOME/.nvm"
[ -s "$NVM_DIR/nvm.sh" ] && \. "$NVM_DIR/nvm.sh"
eval "$(pyenv init -)"
