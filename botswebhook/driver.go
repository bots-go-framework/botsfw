package botswebhook

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"github.com/bots-go-framework/bots-fw/botsfw"
	"github.com/dal-go/dalgo/dal"
	"github.com/strongo/gamp"
	"net/http"
	"runtime/debug"
	"strings"
	"time"
)

// ErrorIcon is used to report errors to user
var ErrorIcon = "🚨"

// BotDriver keeps information about bots and map requests to appropriate handlers
type BotDriver struct {
	Analytics  AnalyticsSettings
	botHost    botsfw.BotHost
	appContext botsfw.BotAppContext
	//router          *WebhooksRouter
	panicTextFooter string
}

var _ botsfw.WebhookDriver = (*BotDriver)(nil) // Ensure BotDriver is implementing interface WebhookDriver

// AnalyticsSettings keeps data for Google Analytics
type AnalyticsSettings struct {
	GaTrackingID string // TODO: Refactor to list of analytics providers
	Enabled      func(r *http.Request) bool
}

// NewBotDriver registers new bot driver (TODO: describe why we need it)
func NewBotDriver(gaSettings AnalyticsSettings, appContext botsfw.BotAppContext, host botsfw.BotHost, panicTextFooter string) BotDriver {
	if appContext == nil {
		panic("appContext == nil")
	}
	if appContext.AppUserCollectionName() == "" {
		panic("appContext.AppUserCollectionName() is empty")
	}
	if host == nil {
		panic("BotHost == nil")
	}
	return BotDriver{
		Analytics:  gaSettings,
		appContext: appContext,
		botHost:    host,
		//router: router,
		panicTextFooter: panicTextFooter,
	}
}

// RegisterWebhookHandlers adds handlers to a bot driver
func (d BotDriver) RegisterWebhookHandlers(httpRouter botsfw.HttpRouter, pathPrefix string, webhookHandlers ...botsfw.WebhookHandler) {
	for _, webhookHandler := range webhookHandlers {
		webhookHandler.RegisterHttpHandlers(d, d.botHost, httpRouter, pathPrefix)
	}
}

// HandleWebhook takes and HTTP request and process it
func (d BotDriver) HandleWebhook(w http.ResponseWriter, r *http.Request, webhookHandler botsfw.WebhookHandler) {

	//c := d.botHost.Context(r)
	c := context.Background()

	handleError := func(err error, message string) {
		log.Errorf(c, "%s: %v", message, err)
		http.Error(w, fmt.Sprintf("%s: %s: %v", http.StatusText(http.StatusInternalServerError), message, err), http.StatusInternalServerError)
	}

	started := time.Now()
	//log.Debugf(c, "BotDriver.HandleWebhook()")
	if w == nil {
		panic("Parameter 'w http.ResponseWriter' is nil")
	}
	if r == nil {
		panic("Parameter 'r *http.Request' is nil")
	}
	if webhookHandler == nil {
		panic("Parameter 'webhookHandler WebhookHandler' is nil")
	}

	botContext, entriesWithInputs, err := webhookHandler.GetBotContextAndInputs(c, r)

	if d.invalidContextOrInputs(c, w, r, botContext, entriesWithInputs, err) {
		return
	}

	log.Debugf(c, "BotDriver.HandleWebhook() => botCode=%v, len(entriesWithInputs): %d", botContext.BotSettings.Code, len(entriesWithInputs))

	var (
		whc               botsfw.WebhookContext // TODO: How do deal with Facebook multiple entries per request?
		measurementSender *gamp.BufferedClient
	)

	var sendStats bool
	{ // Initiate Google Analytics Measurement API client

		if d.Analytics.Enabled == nil {
			sendStats = botContext.BotSettings.Env == botsfw.EnvProduction
			//} else {
			//if sendStats = d.Analytics.Enabled(r); !sendStats {
			//
			//}
			//log.Debugf(c, "d.AnalyticsSettings.Enabled != nil, sendStats: %v", sendStats)
		}
		if sendStats {
			botHost := botContext.BotHost
			measurementSender = gamp.NewBufferedClient("", botHost.GetHTTPClient(c), func(err error) {
				log.Errorf(c, "Failed to log to GA: %v", err)
			})
		} else {
			log.Debugf(c, "botContext.BotSettings.Env=%s, sendStats=%t",
				botContext.BotSettings.Env, sendStats)
		}
	}

	defer func() {
		log.Debugf(c, "driver.deferred(recover) - checking for panic & flush GA")
		if sendStats {
			if d.Analytics.GaTrackingID == "" {
				log.Warningf(c, "driver.Analytics.GaTrackingID is not set")
			} else {
				timing := gamp.NewTiming(time.Since(started))
				timing.TrackingID = d.Analytics.GaTrackingID // TODO: What to do if different FB bots have different Tacking IDs? Can FB handler get messages for different bots? If not (what probably is the case) can we get ID from bot settings instead of driver?
				if err := measurementSender.Queue(timing); err != nil {
					log.Errorf(c, "Failed to log timing to GA: %v", err)
				}
			}
		}

		reportError := func(recovered interface{}) {
			messageText := fmt.Sprintf("Server error (panic): %v\n\n%v", recovered, d.panicTextFooter)
			stack := string(debug.Stack())
			log.Criticalf(c, "Panic recovered: %s\n%s", messageText, stack)

			if sendStats { // Zero if GA is disabled
				d.reportErrorToGA(c, whc, measurementSender, messageText)
			}

			if whc != nil {
				if chatID, err := whc.BotChatID(); err == nil && chatID != "" {
					if responder := whc.Responder(); responder != nil {
						if _, err := responder.SendMessage(c, whc.NewMessage(ErrorIcon+" "+messageText), botsfw.BotAPISendMessageOverResponse); err != nil {
							log.Errorf(c, fmt.Errorf("failed to report error to user: %w", err).Error())
						}
					}
				}
			}
		}

		if recovered := recover(); recovered != nil {
			reportError(recovered)
		} else if sendStats {
			log.Debugf(c, "Flushing GA...")
			if err = measurementSender.Flush(); err != nil {
				log.Warningf(c, "Failed to flush to GA: %v", err)
			} else {
				log.Debugf(c, "Sent to GA: %v items", measurementSender.QueueDepth())
			}
		} else {
			log.Debugf(c, "GA: sendStats=false")
		}
	}()

	//botCoreStores := webhookHandler.CreateBotCoreStores(d.appContext, r)
	//defer func() {
	//	if whc != nil { // TODO: How do deal with Facebook multiple entries per request?
	//		//log.Debugf(c, "Closing BotChatStore...")
	//		//chatData := whc.ChatData()
	//		//if chatData != nil && chatData.GetPreferredLanguage() == "" {
	//		//	chatData.SetPreferredLanguage(whc.DefaultLocale().Code5)
	//		//}
	//	}
	//}()

	for _, entryWithInputs := range entriesWithInputs {
		for i, input := range entryWithInputs.Inputs {
			if input == nil {
				panic(fmt.Sprintf("entryWithInputs.Inputs[%d] == nil", i))
			}
			d.logInput(c, i, input)
			var db dal.DB
			if db, err = botContext.BotSettings.GetDatabase(c); err != nil {
				err = fmt.Errorf("failed to get bot database: %w", err)
				return
			}
			err = db.RunReadwriteTransaction(c, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
				whcArgs := botsfw.NewCreateWebhookContextArgs(r, d.appContext, *botContext, input, tx, measurementSender)
				var err error
				if whc, err = webhookHandler.CreateWebhookContext(whcArgs); err != nil {
					handleError(err, "Failed to create WebhookContext")
					return err
				}
				responder := webhookHandler.GetResponder(w, whc) // TODO: Move inside webhookHandler.CreateWebhookContext()?
				router := botContext.BotSettings.Profile.Router()
				router.Dispatch(webhookHandler, responder, whc) // TODO: Should we return err and handle it here?
				return nil
			})
			if err != nil {
				handleError(err, fmt.Sprintf("Failed to run transaction for entriesWithInputs[%d]", i))
				return
			}
		}
	}
}

func (BotDriver) invalidContextOrInputs(c context.Context, w http.ResponseWriter, r *http.Request, botContext *botsfw.BotContext, entriesWithInputs []botsfw.EntryInputs, err error) bool {
	if err != nil {
		var errAuthFailed botsfw.ErrAuthFailed
		if errors.As(err, &errAuthFailed) {
			log.Warningf(c, "Auth failed: %v", err)
			http.Error(w, http.StatusText(http.StatusForbidden), http.StatusForbidden)
		}
		return true
	}
	if botContext == nil {
		if entriesWithInputs == nil {
			log.Warningf(c, "botContext == nil, entriesWithInputs == nil")
		} else if len(entriesWithInputs) == 0 {
			log.Warningf(c, "botContext == nil, len(entriesWithInputs) == 0")
		} else {
			log.Errorf(c, "botContext == nil, len(entriesWithInputs) == %v", len(entriesWithInputs))
		}
		return true
	} else if entriesWithInputs == nil {
		log.Errorf(c, "entriesWithInputs == nil")
		return true
	}

	switch botContext.BotSettings.Env {
	case botsfw.EnvLocal:
		if !isRunningLocally(r.Host) {
			log.Warningf(c, "whc.GetBotSettings().Mode == Local, host: %v", r.Host)
			w.WriteHeader(http.StatusBadRequest)
			return true
		}
	case botsfw.EnvProduction:
		if isRunningLocally(r.Host) {
			log.Warningf(c, "whc.GetBotSettings().Mode == Production, host: %v", r.Host)
			w.WriteHeader(http.StatusBadRequest)
			return true
		}
	}

	return false
}

func isRunningLocally(host string) bool { // TODO(help-wanted): allow customization
	result := host == "localhost" ||
		strings.HasSuffix(host, ".ngrok.io") ||
		strings.HasSuffix(host, ".ngrok.dev") ||
		strings.HasSuffix(host, ".ngrok.app") ||
		strings.HasSuffix(host, ".ngrok-free.app")
	return result
}

func (BotDriver) reportErrorToGA(c context.Context, whc botsfw.WebhookContext, measurementSender *gamp.BufferedClient, messageText string) {
	log.Warningf(c, "reportErrorToGA() is temporary disabled")

	ga := whc.GA()
	if ga == nil {
		return
	}
	gaMessage := gamp.NewException(messageText, true)
	gaMessage.Common = ga.GaCommon()

	if err := ga.Queue(gaMessage); err != nil {
		log.Errorf(c, "Failed to queue exception message for GA: %v", err)
	} else {
		log.Debugf(c, "Exception message queued for GA.")
	}

	if err := measurementSender.Flush(); err != nil {
		log.Errorf(c, "Failed to flush GA buffer after exception: %v", err)
	} else {
		log.Debugf(c, "GA buffer flushed after exception")
	}
}

func (BotDriver) logInput(c context.Context, i int, input botsfw.WebhookInput) {
	sender := input.GetSender()
	switch input := input.(type) {
	case botsfw.WebhookTextMessage:
		log.Debugf(c, "BotUser#%v(%v %v) => text: %v", sender.GetID(), sender.GetFirstName(), sender.GetLastName(), input.Text())
	case botsfw.WebhookNewChatMembersMessage:
		newMembers := input.NewChatMembers()
		var b bytes.Buffer
		b.WriteString(fmt.Sprintf("NewChatMembers: %d", len(newMembers)))
		for i, member := range newMembers {
			b.WriteString(fmt.Sprintf("\t%d: (%v) - %v %v", i+1, member.GetUserName(), member.GetFirstName(), member.GetLastName()))
		}
		log.Debugf(c, b.String())
	case botsfw.WebhookContactMessage:
		log.Debugf(c, "BotUser#%v(%v %v) => Contact(name: %v|%v, phone number: %v)", sender.GetID(), sender.GetFirstName(), sender.GetLastName(), input.FirstName(), input.LastName(), input.PhoneNumber())
	case botsfw.WebhookCallbackQuery:
		callbackData := input.GetData()
		log.Debugf(c, "BotUser#%v(%v %v) => callback: %v", sender.GetID(), sender.GetFirstName(), sender.GetLastName(), callbackData)
	case botsfw.WebhookInlineQuery:
		log.Debugf(c, "BotUser#%v(%v %v) => inline query: %v", sender.GetID(), sender.GetFirstName(), sender.GetLastName(), input.GetQuery())
	case botsfw.WebhookChosenInlineResult:
		log.Debugf(c, "BotUser#%v(%v %v) => chosen InlineMessageID: %v", sender.GetID(), sender.GetFirstName(), sender.GetLastName(), input.GetInlineMessageID())
	case botsfw.WebhookReferralMessage:
		log.Debugf(c, "BotUser#%v(%v %v) => text: %v", sender.GetID(), sender.GetFirstName(), sender.GetLastName(), input.(botsfw.WebhookTextMessage).Text())
	default:
		log.Warningf(c, "Unhandled input[%v] type: %T", i, input)
	}
}
