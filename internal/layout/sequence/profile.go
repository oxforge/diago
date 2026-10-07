package sequence

import (
	"github.com/oxforge/diago/internal/font"
	"github.com/oxforge/diago/internal/layout"
)

// Profile holds the measurer and every spacing constant the timeline engine
// reads. ScreenProfile reproduces the historical values; TextProfile is the
// cell-multiple configuration for text art.
type Profile struct {
	MeasureText func(text string, sizePt float64, family string) (w, h float64)
	Fonts       LayoutFonts

	ActorPaddingX, ActorPaddingY                     float64
	ActorMinWidth, ActorMinHeight                    float64
	ActorSpacing                                     float64 // minimum center-to-center distance
	ActorBoxGap                                      float64
	InteractionSpacingY                              float64
	SelfLoopWidth, SelfLoopHeight                    float64
	SelfLabelGap                                     float64
	FragmentPaddingX                                 float64
	FragmentPaddingY                                 float64
	FragmentHeaderHeight                             float64
	FragmentFooterHeight                             float64 // extra gap after the last interaction of each fragment that closes there
	SectionLabelPadding                              float64
	DividerInset                                     float64 // divider row sits this far above the section's first interaction
	ActivationWidth                                  float64
	TopMargin, LeftMargin, RightMargin, BottomMargin float64
	LabelAboveOffset                                 float64
	NormalizeMargin                                  float64

	CellW, CellH float64 // > 0 = text mode
}

func (p Profile) TextMode() bool { return p.CellW > 0 && p.CellH > 0 }

// ScreenProfile is the historical configuration every sequence golden was
// generated with.
func ScreenProfile(fonts LayoutFonts) Profile {
	return Profile{
		MeasureText:          font.MeasureText,
		Fonts:                fonts,
		ActorPaddingX:        20,
		ActorPaddingY:        12,
		ActorMinWidth:        80,
		ActorMinHeight:       40,
		ActorSpacing:         120,
		ActorBoxGap:          20,
		InteractionSpacingY:  50,
		SelfLoopWidth:        40,
		SelfLoopHeight:       30,
		SelfLabelGap:         6,
		FragmentPaddingX:     14,
		FragmentPaddingY:     14,
		FragmentHeaderHeight: 24,
		FragmentFooterHeight: 0,
		SectionLabelPadding:  16,
		DividerInset:         7,
		ActivationWidth:      10,
		TopMargin:            20,
		LeftMargin:           40,
		RightMargin:          40,
		BottomMargin:         40,
		LabelAboveOffset:     10,
		NormalizeMargin:      40,
	}
}

// TextProfile is the text art configuration: rune measurement and cell
// multiples everywhere, so every lifeline, header edge, message row,
// activation and fragment edge lies on the 8 × 16 px cell grid.
func TextProfile() Profile {
	return Profile{
		MeasureText:          layout.RuneMeasure,
		Fonts:                LayoutFonts{Family: "Inter", HeaderSize: 14, LabelSize: 12, FragLabelSize: 11},
		ActorPaddingX:        16,
		ActorPaddingY:        16,
		ActorMinWidth:        56,
		ActorMinHeight:       48,
		ActorSpacing:         96,
		ActorBoxGap:          16,
		InteractionSpacingY:  32,
		SelfLoopWidth:        32,
		SelfLoopHeight:       32,
		SelfLabelGap:         8,
		FragmentPaddingX:     8,
		FragmentPaddingY:     16,
		FragmentHeaderHeight: 16,
		FragmentFooterHeight: 16, // one row per closing frame, so the next message never lands on a ╚═╝ line
		SectionLabelPadding:  16,
		DividerInset:         16,
		ActivationWidth:      8,
		TopMargin:            16,
		LeftMargin:           16,
		RightMargin:          16,
		BottomMargin:         32, // one lifeline row past the last message, then the footer
		LabelAboveOffset:     0,
		NormalizeMargin:      16,
		CellW:                8,
		CellH:                16,
	}
}
