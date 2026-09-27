package selfishell

import (
	"bytes"
	"fmt"
	"strings"
)

type blockView struct {
	status     string
	start, end int
	checksum   string
}

func blockContent(name, config string) ([]byte, error) {
	label, comment, body := "", "#", ""
	switch name {
	case "user-zshrc":
		label = "Selfishell initialize"
		body = `if [[ -r "${XDG_CONFIG_HOME:-$HOME/.config}/selfishell/zsh/zshrc" ]]; then
  source "${XDG_CONFIG_HOME:-$HOME/.config}/selfishell/zsh/zshrc"
fi`
	case "user-zprofile":
		label = "Selfishell mise shims"
		body = `if [[ -x "$HOME/.local/bin/mise" ]]; then
  eval "$("$HOME/.local/bin/mise" activate zsh --shims)"
elif command -v mise >/dev/null 2>&1; then
  eval "$(command mise activate zsh --shims)"
fi`
	case "user-zshenv":
		label = "Selfishell zshenv"
		body = "skip_global_compinit=1"
	case "user-ghostty":
		label = "Selfishell ghostty"
		body = "config-file = " + config + `/ghostty/config.ghostty
# To override a Selfishell default above, add it to user.ghostty instead.
config-file = ?user.ghostty`
	case "user-vimrc":
		label = "Selfishell vimrc"
		comment = `"`
		body = `if !empty($XDG_CONFIG_HOME) && filereadable($XDG_CONFIG_HOME . "/selfishell/vim/vimrc")
  source $XDG_CONFIG_HOME/selfishell/vim/vimrc
elseif filereadable(expand("~/.config/selfishell/vim/vimrc"))
  source ~/.config/selfishell/vim/vimrc
endif`
	default:
		return nil, fmt.Errorf("Unknown managed block resource: %s", name)
	}
	return []byte(fmt.Sprintf("%s >>> %s >>>\n%s\n%s <<< %s <<<\n", comment, label, body, comment, label)), nil
}
func inspectBlock(name string, data []byte) (blockView, error) {
	content, err := blockContent(name, "")
	if err != nil {
		return blockView{}, err
	}
	first := bytes.SplitN(content, []byte("\n"), 2)[0]
	last := bytes.Split(content, []byte("\n"))
	endMarker := last[len(last)-2]
	var beginCount, endCount, related, start, finish, offset int
	for _, line := range bytes.Split(data, []byte("\n")) {
		if bytes.Equal(line, first) {
			beginCount++
			start = offset
		}
		offset += len(line) + 1
		if bytes.Equal(line, endMarker) {
			endCount++
			finish = offset
		}
	}
	markerLabel := string(first)
	if pos := strings.Index(markerLabel, " >>> "); pos >= 0 {
		markerLabel = markerLabel[pos+5 : len(markerLabel)-4]
	}
	for _, line := range bytes.Split(data, []byte("\n")) {
		if bytes.Contains(line, []byte(markerLabel)) {
			related++
		}
	}
	if beginCount == 0 && endCount == 0 && related == 0 {
		return blockView{status: "absent"}, nil
	}
	if beginCount != 1 || endCount != 1 || related != 2 || finish <= start || finish > len(data) {
		return blockView{status: "malformed"}, nil
	}
	checksum, err := checksumBytes(data[start:finish])
	return blockView{status: "intact", start: start, end: finish, checksum: checksum}, err
}
func spliceBlock(data []byte, view blockView, replacement []byte) []byte {
	result := append([]byte{}, data[:view.start]...)
	result = append(result, replacement...)
	return append(result, data[view.end:]...)
}
