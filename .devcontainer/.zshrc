export ZSH="${HOME}/.oh-my-zsh"

ZSH_THEME="refined"
ZSH_CUSTOM="${ZSH}/custom"

plugins=(git zsh-autosuggestions zsh-syntax-highlighting)
fpath+="${ZSH_CUSTOM}/plugins/zsh-completions/src"

source "${ZSH}/oh-my-zsh.sh"
