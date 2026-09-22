package shellcmd

// cmd_direction.go — the surface for FolderData.Mode.
//
// A model something reads but nothing can set is the read-only-surface
// violation from the other side (AP57): S6 made the reconciler branch on
// Mode, and without a verb the operator would have a setting that
// governs their bytes and no way to choose it. The whole operation is
// `SetFolderMode` in mode_op.go, so this file is printing and nothing
// else — the panel calls the same function rather than reimplementing
// the rules in the renderer.

import (
	"fmt"

	"entity-workbench-go/workbench"
)

func cmdDirection(sh *Shell, args []string) (Result, error) {
	if sh == nil || sh.ShellWorkspace == nil {
		return Result{}, fmt.Errorf("no workspace")
	}
	if len(args) == 0 {
		return directionListing(sh)
	}
	if len(args) < 2 {
		return Result{}, fmt.Errorf(
			"usage: direction <folder-id> <send|receive|both>   " +
				"(run `direction` with no arguments to list the folders and their ids)")
	}
	res, err := sh.SetFolderMode(SetFolderModeRequest{FolderID: args[0], Mode: args[1]})
	if err != nil {
		return Result{}, err
	}

	lines := []string{}
	if !res.Changed {
		lines = append(lines, fmt.Sprintf("%s is already %s — nothing changed", res.Label, res.Mode))
	} else {
		lines = append(lines,
			fmt.Sprintf("%s: %s -> %s", res.Label, res.Previous, res.Mode),
			"", directionMeaning(res.Mode))
	}
	if res.Caveat != "" {
		lines = append(lines, "", res.Caveat)
	}
	return LinesResult(lines), nil
}

// directionListing prints every declared folder with its id and current
// direction, because the id is what the verb takes and it is not a name
// an operator would guess.
func directionListing(sh *Shell) (Result, error) {
	folders, problems := workbench.LoadFolders(sh.Local.Peer.Store())
	if len(folders) == 0 && len(problems) == 0 {
		return MessageResult("no folders are declared on this peer"), nil
	}
	lines := []string{fmt.Sprintf("declared folders: %d", len(folders)), ""}
	for _, f := range folders {
		owner := "ours"
		if !f.IsLocal() {
			owner = "from " + sh.DeviceLabelFor(f.Origin)
		}
		lines = append(lines, fmt.Sprintf("  %-12s %-8s %s", f.EffectiveMode(), owner, f.DisplayLabel()))
		lines = append(lines, fmt.Sprintf("  %-12s %s", "", f.ID))
	}
	if len(problems) > 0 {
		lines = append(lines, "", "records that did not decode:")
		for _, p := range problems {
			lines = append(lines, "  "+p)
		}
	}
	lines = append(lines, "", "change one with: direction <folder-id> <send|receive|both>")
	return LinesResult(lines), nil
}

func directionMeaning(mode string) string {
	switch mode {
	case workbench.FolderModeSend:
		return "changes here are published to them; theirs are not applied here."
	case workbench.FolderModeReceive:
		return "their changes are applied here; nothing here is published to them."
	default:
		return "changes flow both ways — a shared folder in the ordinary sense."
	}
}
