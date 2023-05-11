package botsfw

import (
	"fmt"
	"github.com/bots-go-framework/bots-fw-store/botsfwdal"
	"github.com/bots-go-framework/bots-fw-store/botsfwmodels"
	//"github.com/dal-go/dalgo/dal"
	"github.com/strongo/i18n"
	"net/http"
	"net/url"
	"strings"
	"time"

	"context"
	"github.com/strongo/app"
	"github.com/strongo/gamp"
)

//var _ WebhookContext = (*WebhookContextBase)(nil)

// WebhookContextBase provides base implementation of WebhookContext interface
// TODO: Document purpose of a dedicated base struct (e.g. example of usage by developers)
type WebhookContextBase struct {
	//w          http.ResponseWriter
	r             *http.Request
	c             context.Context
	botAppContext BotAppContext
	botContext    BotContext // TODO: rename to something strongo
	botPlatform   BotPlatform
	input         WebhookInput
	//recordsMaker        botsfwmodels.BotRecordsMaker
	recordsFieldsSetter BotRecordsFieldsSetter

	isInGroup func() bool

	getLocaleAndChatID func() (locale, chatID string, err error) // TODO: Document why we need to pass context. Is it to support transactions?

	locale i18n.Locale

	chatID   string
	chatData botsfwmodels.ChatData

	// BotUserID is a user ID in bot's platform.
	// Telegram has it is an integer, but we keep it as a string for consistency & simplicity.
	BotUserID string

	//
	appUserData botsfwmodels.AppUserData

	i18n.Translator
	//Locales    strongo.LocalesProvider

	dal botsfwdal.DataAccess

	gaContext gaContext
}

func (whcb *WebhookContextBase) RecordsFieldsSetter() BotRecordsFieldsSetter {
	return whcb.recordsFieldsSetter
}

func (whcb *WebhookContextBase) Store() botsfwdal.DataAccess {
	return whcb.dal
}

func (whcb *WebhookContextBase) BotContext() BotContext {
	return whcb.botContext
}

// SetChatID sets chat ID
func (whcb *WebhookContextBase) SetChatID(v string) {
	whcb.chatID = v
}

// LogRequest logs request data to logging system
func (whcb *WebhookContextBase) LogRequest() {
	whcb.input.LogRequest()
}

// // RunReadwriteTransaction starts a transaction. This needed to coordinate application & framework changes.
//func (whcb *WebhookContextBase) RunReadwriteTransaction(c context.Context, f func(ctx context.Context)) error {
//	db, err := whcb.botContext.BotHost.DB(c)
//	if err != nil {
//		return err
//	}
//	return db.RunReadwriteTransaction(c, f, options...)
//}

//
//// RunReadonlyTransaction starts a readonly transaction.
//func (whcb *WebhookContextBase) RunReadonlyTransaction(c context.Context, f dal.ROTxWorker, options ...dal.TransactionOption) error {
//	db, err := whcb.botContext.BotHost.DB(c)
//	if err != nil {
//		return err
//	}
//	return db.RunReadonlyTransaction(c, f, options...)
//}

// IsInTransaction detects if request is within a transaction
func (whcb *WebhookContextBase) IsInTransaction(context.Context) bool {
	panic("not implemented")
	//return whcb.botContext.BotHost.DB().IsInTransaction(c)
}

// NonTransactionalContext creates a non transaction context for operations that needs to be executed outside of transaction.
func (whcb *WebhookContextBase) NonTransactionalContext(context.Context) context.Context {
	panic("not implemented")
	//return whcb.botContext.BotHost.DB().NonTransactionalContext(tc)
}

// Request returns reference to current HTTP request
func (whcb *WebhookContextBase) Request() *http.Request {
	return whcb.r
}

// Environment defines current environment (PROD, DEV, LOCAL, etc)
func (whcb *WebhookContextBase) Environment() strongo.Environment {
	return whcb.botContext.BotSettings.Env
}

// MustBotChatID returns bot chat ID and panic if missing it
func (whcb *WebhookContextBase) MustBotChatID() (chatID string) {
	var err error
	if chatID, err = whcb.BotChatID(); err != nil {
		panic(err)
	} else if chatID == "" {
		panic("BotChatID() returned an empty string")
	}
	return
}

// BotChatID returns bot chat ID
func (whcb *WebhookContextBase) BotChatID() (botChatID string, err error) {
	if whcb.chatID != "" {
		return whcb.chatID, nil
	}
	//log.Debugf(whcb.c, "*WebhookContextBase.BotChatID()")

	input := whcb.Input()
	if botChatID, err = input.BotChatID(); err != nil {
		return
	} else if botChatID != "" {
		whcb.chatID = botChatID
		return whcb.chatID, nil
	}
	if whcb.getLocaleAndChatID != nil {
		if _, botChatID, err = whcb.getLocaleAndChatID(); err != nil {
			return
		}
		if botChatID != "" {
			whcb.chatID = botChatID
			return
		}
	}
	switch input := input.(type) {
	case WebhookCallbackQuery:
		data := input.GetData()
		if strings.Contains(data, "chat=") {
			values, err := url.ParseQuery(data)
			if err != nil {
				return "", fmt.Errorf("failed to GetData() from webhookInput.InputCallbackQuery(): %w", err)
			}
			whcb.chatID = values.Get("chat")
		}
	case WebhookInlineQuery:
		// pass
	case WebhookChosenInlineResult:
		// pass
	default:
		whcb.LogRequest()
		log.Debugf(whcb.c, "BotChatID(): *.WebhookContextBaseBotChatID(): Unhandled input type: %T", input)
	}

	return whcb.chatID, nil
}

// AppUserID return current app user ID as a string. AppUserIntID() is deprecated.
func (whcb *WebhookContextBase) AppUserID() (appUserID string) {
	if !whcb.isInGroup() {
		if chatEntity := whcb.ChatEntity(); chatEntity != nil {
			appUserID = chatEntity.GetAppUserID()
		}
	}
	if appUserID == "" {
		if botUser, err := whcb.GetOrCreateBotUserEntityBase(); err != nil {
			panic(fmt.Errorf("failed to get bot user entity: %w", err))
		} else {
			appUserID = botUser.GetAppUserID()
		}
	}
	return
}

// GetAppUser loads information about current app user from persistent storage
func (whcb *WebhookContextBase) GetAppUser() (botsfwmodels.AppUserData, error) { // TODO: Can/should this be cached?
	appUserID := whcb.AppUserID()
	botID := whcb.GetBotCode()
	appUser := whcb.BotAppContext().NewBotAppUserEntity()
	err := whcb.dal.GetAppUserByID(whcb.Context(), botID, appUserID, appUser)
	return appUser, err
}

// ExecutionContext returns an execution context for strongo app
func (whcb *WebhookContextBase) ExecutionContext() strongo.ExecutionContext {
	return whcb
}

// BotAppContext returns bot app context
func (whcb *WebhookContextBase) BotAppContext() BotAppContext {
	return whcb.botAppContext
}

// IsInGroup signals if the bot request is send within group chat
func (whcb *WebhookContextBase) IsInGroup() bool {
	return whcb.isInGroup()
}

// NewWebhookContextBase creates base bot context
func NewWebhookContextBase(
	r *http.Request,
	botAppContext BotAppContext,
	botPlatform BotPlatform,
	botContext BotContext,
	webhookInput WebhookInput,
	botCoreStores botsfwdal.DataAccess,
	gaMeasurement GaQueuer,
	isInGroup func() bool,
	getLocaleAndChatID func(c context.Context) (locale, chatID string, err error),
) *WebhookContextBase {
	if r == nil {
		panic("r == nil")
	}
	c := botContext.BotHost.Context(r)
	whcb := WebhookContextBase{
		r: r,
		c: c,
		getLocaleAndChatID: func() (locale, chatID string, err error) {
			return getLocaleAndChatID(c)
		},
		botAppContext: botAppContext,
		botPlatform:   botPlatform,
		botContext:    botContext,
		input:         webhookInput,
		isInGroup:     isInGroup,
		dal:           botCoreStores,
	}
	whcb.gaContext = gaContext{
		whcb:          &whcb,
		gaMeasurement: gaMeasurement,
	}
	if isInGroup() && whcb.getLocaleAndChatID != nil {
		if locale, chatID, err := whcb.getLocaleAndChatID(); err != nil {
			panic(err)
		} else {
			if chatID != "" {
				whcb.chatID = chatID
			}
			if locale != "" {
				if err := whcb.SetLocale(locale); err != nil {
					log.Errorf(c, "Failed to set locale: %v", err)
				}
			}
		}
	}
	whcb.Translator = botAppContext.GetTranslator(whcb.c)
	return &whcb
}

// Input returns webhook intput
func (whcb *WebhookContextBase) Input() WebhookInput {
	return whcb.input
}

// Chat returns webhook chat
func (whcb *WebhookContextBase) Chat() WebhookChat { // TODO: remove
	return whcb.input.Chat()
}

// GetRecipient returns receiver of the message
func (whcb *WebhookContextBase) GetRecipient() WebhookRecipient { // TODO: remove
	return whcb.input.GetRecipient()
}

// GetSender returns sender of the message
func (whcb *WebhookContextBase) GetSender() WebhookSender { // TODO: remove
	return whcb.input.GetSender()
}

// GetTime returns time of the message
func (whcb *WebhookContextBase) GetTime() time.Time { // TODO: remove
	return whcb.input.GetTime()
}

// InputType returns input type
func (whcb *WebhookContextBase) InputType() WebhookInputType { // TODO: remove
	return whcb.input.InputType()
}

// GaMeasurement returns a provider to send information to Google Analytics
func (gac gaContext) GaMeasurement() GaQueuer {
	return gac.gaMeasurement
}

type gaContext struct {
	whcb          *WebhookContextBase
	gaMeasurement GaQueuer
}

// GA provides interface to Google Analytics
func (whcb *WebhookContextBase) GA() GaContext {
	return whcb.gaContext
}

func (gac gaContext) Queue(message gamp.Message) error {
	if gac.gaMeasurement == nil { // TODO: not good :(
		return nil
	}
	if message.GetTrackingID() == "" {
		message.SetTrackingID(gac.whcb.GetBotSettings().GAToken)
		if message.GetTrackingID() == "" {
			return fmt.Errorf("gaContext.Queue(%v): %w", message, gamp.ErrNoTrackingID)
		}
	}
	return gac.gaMeasurement.Queue(message)
}

//	func (gac gaContext) Flush() error {
//		return gac.gaMeasurement.
//	}
//
// GaCommon creates context for Google Analytics
func (gac gaContext) GaCommon() gamp.Common {
	whcb := gac.whcb
	if whcb.chatData != nil {
		return gamp.Common{
			UserID:       whcb.chatData.GetAppUserID(),
			UserLanguage: strings.ToLower(whcb.chatData.GetPreferredLanguage()),
			//ClientID:      whcb.chatData.GetGaClientID(), // TODO: Restore feature
			ApplicationID: fmt.Sprintf("bot.%v.%v", whcb.botPlatform.ID(), whcb.GetBotCode()),
			UserAgent:     fmt.Sprintf("%v bot @ %v", whcb.botPlatform.ID(), whcb.r.Host),
			DataSource:    "bot",
		}
	}
	return gamp.Common{
		DataSource: "bot",
		ClientID:   "", // TODO: DO NOT USE hardcoded value here!
	}
}

func (gac gaContext) GaEvent(category, action string) *gamp.Event { // TODO: remove
	return gamp.NewEvent(category, action, gac.GaCommon())
}

func (gac gaContext) GaEventWithLabel(category, action, label string) *gamp.Event {
	return gamp.NewEventWithLabel(category, action, label, gac.GaCommon())
}

// BotPlatform inidates on which bot platform we process message
func (whcb *WebhookContextBase) BotPlatform() BotPlatform {
	return whcb.botPlatform
}

// GetBotSettings settings of the current bot
func (whcb *WebhookContextBase) GetBotSettings() BotSettings {
	return whcb.botContext.BotSettings
}

// GetBotCode returns current bot code
func (whcb *WebhookContextBase) GetBotCode() string {
	return whcb.botContext.BotSettings.Code
}

// GetBotToken returns current bot API token
func (whcb *WebhookContextBase) GetBotToken() string {
	return whcb.botContext.BotSettings.Token
}

// Translate translates string
func (whcb *WebhookContextBase) Translate(key string, args ...interface{}) string {
	return whcb.Translator.Translate(key, whcb.Locale().Code5, args...)
}

// TranslateNoWarning translates string without warnings
func (whcb *WebhookContextBase) TranslateNoWarning(key string, args ...interface{}) string {
	return whcb.Translator.TranslateNoWarning(key, whcb.locale.Code5, args...)
}

//func (whcb *WebhookContextBase) GetHTTPClient() *http.Client {
//	return whcb.botContext.BotHost.GetHTTPClient(whcb.c)
//}

// HasChatEntity return true if messages is within chat
func (whcb *WebhookContextBase) HasChatEntity() bool {
	return whcb.chatData != nil
}

//func (whcb *WebhookContextBase) SaveAppUser(appUserID int64, appUserEntity BotAppUser) error {
//	return whcb.BotAppUserStore.SaveAppUser(whcb.Context(), appUserID, appUserEntity)
//}

// SetChatEntity sets app entity for the context (loaded from DB)
func (whcb *WebhookContextBase) SetChatEntity(chatEntity botsfwmodels.ChatData) {
	whcb.chatData = chatEntity
}

// ChatEntity returns app entity for the context (loaded from DB)
func (whcb *WebhookContextBase) ChatEntity() botsfwmodels.ChatData {
	if whcb.chatData != nil {
		return whcb.chatData
	}
	//panic("*WebhookContextBase.ChatData()")
	//log.Debugf(whcb.c, "*WebhookContextBase.ChatData()")
	chatID, err := whcb.BotChatID()
	if err != nil {
		panic(fmt.Errorf("failed to call whcb.BotChatID(): %w", err))
	}
	if chatID == "" {
		log.Debugf(whcb.c, "whcb.BotChatID() is empty string")
		return nil
	}
	if err := whcb.loadChatEntityBase(); err != nil {
		if botsfwdal.IsNotFoundErr(err) {
			botID := whcb.GetBotCode()
			if whcb.recordsFieldsSetter == nil {
				panic("whcb.recordsFieldsSetter == nil")
			}
			sender := whcb.input.GetSender()
			botUserID := fmt.Sprintf("%v", sender.GetID())
			appUserID := whcb.AppUserID()
			webhookChat := whcb.Chat()
			isAccessGranted := true // TODO: Implement!!!
			if err = whcb.recordsFieldsSetter.SetBotChatFields(whcb.chatData, botID, botUserID, appUserID, webhookChat, isAccessGranted); err != nil {
				panic(fmt.Errorf("failed to call whcb.recordsMaker.MakeBotChatDto(): %w", err))
			}
		} else {
			panic(fmt.Errorf("failed to call whcb.getChatEntityBase(): %w", err))
		}
	}
	return whcb.chatData
}

// GetOrCreateBotUserEntityBase to be documented
func (whcb *WebhookContextBase) GetOrCreateBotUserEntityBase() (botsfwmodels.BotUser, error) {
	c := whcb.Context()
	log.Debugf(c, "GetOrCreateBotUserEntityBase()")
	sender := whcb.input.GetSender()
	botID := whcb.GetBotCode()
	botUserID := fmt.Sprintf("%v", sender.GetID())
	botUser, err := whcb.dal.GetBotUserByID(c, botID, botUserID)

	if err != nil {
		if !botsfwdal.IsNotFoundErr(err) {
			log.Infof(c, "Bot user entity not found, creating a new one...")
			appUserID := whcb.AppUserID()
			var botUserDto botsfwmodels.BotUser
			if err = whcb.recordsFieldsSetter.SetBotUserFields(botUser, botID, appUserID, botUserID, sender); err != nil {
				log.Errorf(c, "WebhookContextBase.GetOrCreateBotUserEntityBase(): failed to make bot user DTO: %v", err)
				return nil, err
			}
			if err = whcb.dal.SaveBotUser(c, botID, botUserID, botUserDto); err != nil {
				log.Errorf(c, "WebhookContextBase.GetOrCreateBotUserEntityBase(): failed to create bot user: %v", err)
				return nil, err
			}
			log.Infof(c, "Bot user entity created")

			ga := whcb.gaContext
			if err = ga.Queue(ga.GaEvent("users", "user-created")); err != nil { //TODO: Should be outside
				log.Errorf(c, "Failed to queue GA event: %v", err)
			}

			if err = ga.Queue(ga.GaEventWithLabel("users", "messenger-linked", whcb.botPlatform.ID())); err != nil { // TODO: Should be outside
				log.Errorf(c, "Failed to queue GA event: %v", err)
			}

			if whcb.GetBotSettings().Env == strongo.EnvProduction {
				if err = ga.Queue(ga.GaEventWithLabel("bot-users", "bot-user-created", whcb.botPlatform.ID())); err != nil {
					log.Errorf(c, "Failed to queue GA event: %v", err)
				}
			}
		}
		return nil, err
	} else {
		log.Infof(c, "Found existing bot user entity")
	}
	return botUser, err
}

func (whcb *WebhookContextBase) loadChatEntityBase() (err error) {
	c := whcb.Context()
	if whcb.HasChatEntity() {
		log.Warningf(c, "Duplicate call of func (whc *bot.WebhookContext) _getChat()")
		return nil
	}

	var chatKey = botsfwmodels.ChatKey{
		BotID: whcb.GetBotCode(),
	}
	if chatKey.ChatID, err = whcb.BotChatID(); err != nil {
		return fmt.Errorf("failed to call whcb.BotChatID(): %w", err)
	}

	//log.Debugf(c, "loadChatEntityBase(): getLocaleAndChatID: %v", botChatID)
	botChatStore := whcb.dal
	if botChatStore == nil {
		panic("botChatStore == nil")
	}
	botChatEntity, err := botChatStore.GetBotChatData(c, chatKey)
	if err != nil {
		if !botsfwdal.IsNotFoundErr(err) {
			return err
		}
		err = nil
		log.Infof(c, "BotChat not found, first check for bot user entity...")
		botUser, err := whcb.GetOrCreateBotUserEntityBase()
		if err != nil {
			return err
		}

		botUserID := fmt.Sprintf("%v", whcb.input.GetSender().GetID())

		isAccessGranted := botUser.IsAccessGranted()
		whChat := whcb.input.Chat()
		appUserID := botUser.GetAppUserID()
		err = whcb.recordsFieldsSetter.SetBotChatFields(botChatEntity, chatKey.BotID, botUserID, appUserID, whChat, isAccessGranted)

		if whcb.GetBotSettings().Env == strongo.EnvProduction {
			ga := whcb.gaContext
			if err := ga.Queue(ga.GaEventWithLabel("bot-chats", "bot-chat-created", whcb.botPlatform.ID())); err != nil {
				log.Errorf(c, "Failed to queue GA event: %v", err)
			}
		}

	}

	if sender := whcb.input.GetSender(); sender != nil {
		if languageCode := sender.GetLanguage(); languageCode != "" {
			botChatEntity.AddClientLanguage(languageCode)
		}
	}

	if chatLocale := botChatEntity.GetPreferredLanguage(); chatLocale != "" && chatLocale != whcb.locale.Code5 {
		if err = whcb.SetLocale(chatLocale); err != nil {
			log.Errorf(c, "failed to set locate: %v", err)
		}
	}
	whcb.chatData = botChatEntity
	return err
}

// AppUserEntity current app user entity from data storage
func (whcb *WebhookContextBase) AppUserEntity() botsfwmodels.AppUserData {
	return whcb.appUserData
}

// Context for current request
func (whcb *WebhookContextBase) Context() context.Context {
	return whcb.c
}

// SetContext sets current context // TODO: explain why we need this as probably should be in constructor?
func (whcb *WebhookContextBase) SetContext(c context.Context) {
	whcb.c = c
}

// NewMessageByCode creates new translated message by i18n code
func (whcb *WebhookContextBase) NewMessageByCode(messageCode string, a ...interface{}) (m MessageFromBot) {
	return whcb.NewMessage(fmt.Sprintf(whcb.Translate(messageCode), a...))
}

// MessageText returns text of received message
func (whcb *WebhookContextBase) MessageText() string {
	if tm, ok := whcb.Input().(WebhookTextMessage); ok {
		return tm.Text()
	}
	return ""
}

// NewMessage creates new message from bot
func (whcb *WebhookContextBase) NewMessage(text string) (m MessageFromBot) {
	m.Text = text
	m.Format = MessageFormatHTML
	return
}

// Locale indicates current language
func (whcb *WebhookContextBase) Locale() i18n.Locale {
	if whcb.locale.Code5 == "" {
		if chatEntity := whcb.ChatEntity(); chatEntity != nil {
			if locale := chatEntity.GetPreferredLanguage(); locale != "" {
				if err := whcb.SetLocale(locale); err == nil {
					return whcb.locale
				}
			}
		}
		whcb.locale = whcb.botContext.BotSettings.Locale
	}
	return whcb.locale
}

// SetLocale sets current language
func (whcb *WebhookContextBase) SetLocale(code5 string) error {
	locale, err := whcb.botAppContext.SupportedLocales().GetLocaleByCode5(code5)
	if err != nil {
		log.Errorf(whcb.c, "*WebhookContextBase.SetLocate(%v) - %v", code5, err)
		return err
	}
	whcb.locale = locale
	log.Debugf(whcb.Context(), "*WebhookContextBase.SetLocale(%v) => Done", code5)
	return nil
}
