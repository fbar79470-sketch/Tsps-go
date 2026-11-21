// DialogueExpression is the chathead animation id a speaker wears while
// talking: DialogueExpression (dialogues/DialogueExpression.ts:1-45).
// TS is a class wrapping one number, with static instances; Go is an int
// type so TS's `number | DialogueExpression` expression parameters
// (NpcDialogue.ts:13, PlayerDialogue.ts:15) collapse to one type — an
// arbitrary animation id is DialogueExpression(id), and the named
// expressions below are the TS static instances, with identical values
// (note the non-monotonic ids: SAD is 599, NEAR_TEARS 598, LAUGHING_2
// 608 above LONGER_LAUGHING 606 — transcribed, not sorted).
package game

// DialogueExpression is a chathead animation id.
type DialogueExpression int

// ID returns the animation id (DialogueExpression.getExpression,
// DialogueExpression.ts:39-41).
func (e DialogueExpression) ID() int { return int(e) }

// Named expressions (DialogueExpression.ts:2-31).
const (
	DialogueExpressionHappy               DialogueExpression = 588
	DialogueExpressionCalm                DialogueExpression = 589
	DialogueExpressionCalmContinued       DialogueExpression = 590
	DialogueExpressionDefault             DialogueExpression = 591
	DialogueExpressionEvil                DialogueExpression = 592
	DialogueExpressionEvilContinued       DialogueExpression = 593
	DialogueExpressionDelightedEvil       DialogueExpression = 594
	DialogueExpressionAnnoyed             DialogueExpression = 595
	DialogueExpressionDistressed          DialogueExpression = 596
	DialogueExpressionDistressedContinued DialogueExpression = 597
	DialogueExpressionDisorientedLeft     DialogueExpression = 600
	DialogueExpressionDisorientedRight    DialogueExpression = 601
	DialogueExpressionUninterested        DialogueExpression = 602
	DialogueExpressionSleepy              DialogueExpression = 603
	DialogueExpressionPlainEvil           DialogueExpression = 604
	DialogueExpressionLaughing            DialogueExpression = 605
	DialogueExpressionLaughing2           DialogueExpression = 608
	DialogueExpressionLongerLaughing      DialogueExpression = 606
	DialogueExpressionLongerLaughing2     DialogueExpression = 607
	DialogueExpressionEvilLaughShort      DialogueExpression = 609
	DialogueExpressionSlightlySad         DialogueExpression = 610
	DialogueExpressionSad                 DialogueExpression = 599
	DialogueExpressionVerySad             DialogueExpression = 611
	DialogueExpressionOther               DialogueExpression = 612
	DialogueExpressionNearTears           DialogueExpression = 598
	DialogueExpressionNearTears2          DialogueExpression = 613
	DialogueExpressionAngry1              DialogueExpression = 614
	DialogueExpressionAngry2              DialogueExpression = 615
	DialogueExpressionAngry3              DialogueExpression = 616
	DialogueExpressionAngry4              DialogueExpression = 617
)
