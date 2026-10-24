package ui

import (
	"github.com/charmbracelet/lipgloss"

	"github.com/jzills/snoop/internal/parse"
)

// Adaptive palette: {Light, Dark}.
var (
	cAccent   = lipgloss.AdaptiveColor{Light: "#5B4BDB", Dark: "#A99CFF"}
	cText     = lipgloss.AdaptiveColor{Light: "#1F2328", Dark: "#E6E6E6"}
	cDim      = lipgloss.AdaptiveColor{Light: "#6E7781", Dark: "#7D8590"}
	cFaint    = lipgloss.AdaptiveColor{Light: "#AFB8C1", Dark: "#3D444D"}
	cSelBg    = lipgloss.AdaptiveColor{Light: "#E7E3FF", Dark: "#2B2840"}
	cPromptBg = lipgloss.AdaptiveColor{Light: "#F3F1FF", Dark: "#1D1B2B"}
	cBuiltin  = lipgloss.AdaptiveColor{Light: "#0969DA", Dark: "#6CB6FF"}
	cSkill    = lipgloss.AdaptiveColor{Light: "#BF3989", Dark: "#F778BA"}
	cMCP      = lipgloss.AdaptiveColor{Light: "#1A7F64", Dark: "#3FD1B0"}
	cAgent    = lipgloss.AdaptiveColor{Light: "#9A6700", Dark: "#E3B341"}
	cErr      = lipgloss.AdaptiveColor{Light: "#CF222E", Dark: "#FF7B72"}
	cOK       = lipgloss.AdaptiveColor{Light: "#1A7F37", Dark: "#56D364"}
	cAdd      = lipgloss.AdaptiveColor{Light: "#1A7F37", Dark: "#56D364"}
	cDel      = lipgloss.AdaptiveColor{Light: "#CF222E", Dark: "#FF7B72"}
)

var (
	sText     = lipgloss.NewStyle().Foreground(cText)
	sDim      = lipgloss.NewStyle().Foreground(cDim)
	sFaint    = lipgloss.NewStyle().Foreground(cFaint)
	sAccent   = lipgloss.NewStyle().Foreground(cAccent)
	sBold     = lipgloss.NewStyle().Bold(true).Foreground(cText)
	sThinking = lipgloss.NewStyle().Foreground(cDim).Italic(true)
	sPrompt   = lipgloss.NewStyle().Foreground(cAccent).Bold(true)
	sErr      = lipgloss.NewStyle().Foreground(cErr)
	sOK       = lipgloss.NewStyle().Foreground(cOK)
	sAdd      = lipgloss.NewStyle().Foreground(cAdd)
	sDel      = lipgloss.NewStyle().Foreground(cDel)

	sTitle     = lipgloss.NewStyle().Bold(true).Foreground(cAccent)
	sTabOn     = lipgloss.NewStyle().Bold(true).Foreground(cAccent).Underline(true)
	sTabOff    = lipgloss.NewStyle().Foreground(cDim)
	sKey       = lipgloss.NewStyle().Foreground(cAccent)
	sSection   = lipgloss.NewStyle().Bold(true).Foreground(cAccent).MarginTop(1)
	sLive      = lipgloss.NewStyle().Bold(true).Foreground(cOK)
	sPaneTitle = lipgloss.NewStyle().Bold(true).Foreground(cText)
)

func kindColor(k parse.ToolKind) lipgloss.AdaptiveColor {
	switch k {
	case parse.ToolSkill:
		return cSkill
	case parse.ToolMCP:
		return cMCP
	case parse.ToolAgent:
		return cAgent
	}
	return cBuiltin
}

func kindGlyph(k parse.ToolKind) string {
	switch k {
	case parse.ToolSkill:
		return "✦"
	case parse.ToolMCP:
		return "⇄"
	case parse.ToolAgent:
		return "⤷"
	}
	return "▸"
}

const (
	glyphUser     = "❯"
	glyphText     = "◆"
	glyphThinking = "∴"
	glyphSystem   = "·"
	glyphCompact  = "⟳"
	glyphOK       = "✓"
	glyphErr      = "✗"
	glyphPending  = "◌"
)
