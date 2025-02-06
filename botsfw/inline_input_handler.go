package botsfw

import "github.com/bots-go-framework/bots-fw/botinput"

// TODO: Most likely this should not need to be a part of the botsfw package

// InlineQueryHandlerFunc defines a function that handles inline query
type InlineQueryHandlerFunc func(whc WebhookContext, inlineQuery botinput.WebhookInlineQuery) (handled bool, m MessageFromBot, err error)

// ChosenInlineResultHandlerFunc defines a function that handles chosen inline result
type ChosenInlineResultHandlerFunc func(whc WebhookContext, inlineQuery botinput.WebhookChosenInlineResult) (handled bool, m MessageFromBot, err error)

// InlineInputHandler defines handlers to deal with inline inputs
type InlineInputHandler struct { // This should
	ProfileID                string // Not sure if we really need it
	HandleInlineQuery        InlineQueryHandlerFunc
	HandleChosenInlineResult ChosenInlineResultHandlerFunc
}
