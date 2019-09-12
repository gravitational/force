package slack

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/gravitational/force"

	"github.com/gravitational/trace"
	"github.com/nlopes/slack"
)

type convoPhase string

const (
	phaseInit      = convoPhase("init")
	phaseConfirm   = convoPhase("confirm")
	phaseConfirmed = convoPhase("confirmed")
)

type convoState struct {
	phase convoPhase
	// chatToConfirm puts chat bot in a state expecting to confirm a command
	// before executing
	chatToConfirm *Chat
	// confirmedChat is set whenever chat has been confirmed
	confirmedChat *Chat
}

type conversation struct {
	lock            *sync.RWMutex
	ctx             context.Context
	threadTimestamp string
	bot             *bot
	state           convoState
	channel         string
	userID          string
}

func (c *conversation) setState(state convoState) {
	c.lock.Lock()
	defer c.lock.Unlock()
	c.state = state
}

func (c *conversation) getState() convoState {
	c.lock.Lock()
	defer c.lock.Unlock()
	return c.state
}

func (c *conversation) handleMessage(message string) {
	message = strings.TrimSpace(strings.ReplaceAll(message, mention(c.userID), ""))
	state := c.getState()
	switch state.phase {
	case phaseInit:
		chat, err := c.bot.parser.Parse(message)
		if err != nil {
			msg := force.Capitalize(err.Error() + ".")
			if trace.IsNotFound(err) {
				c.sendMessage(msg + "\n" + c.bot.parser.HelpMessage(""))
			} else {
				c.sendMessage(msg)
			}
			return
		}
		if chat.RequestedHelp {
			c.sendMessage(c.bot.parser.HelpMessage(chat.Command.Name))
			return
		}
		if chat.Command.Confirm {
			c.setState(convoState{
				phase:         phaseConfirm,
				chatToConfirm: chat,
			})
			c.sendMessage(chat.Command.ConfirmationMessage(chat.Values))
			return
		}
		c.setState(convoState{
			phase:         phaseConfirmed,
			confirmedChat: chat,
		})
		if err := c.sendEvent(chat.Values); err != nil {
			c.sendMessage("Failed to confirm command - internal error. Check logs for details.")
			c.bot.log.WithError(err).Errorf("Failed to send event.")
			return
		}
		c.sendMessage(
			fmt.Sprintf("Executing command: %v with values %v", chat.Command.Name, chat.Values))
		return
	case phaseConfirm:
		if Confirm(message) {
			c.setState(convoState{
				phase:         phaseConfirmed,
				confirmedChat: state.chatToConfirm,
			})
			if err := c.sendEvent(state.chatToConfirm.Values); err != nil {
				c.bot.log.WithError(err).Errorf("Failed to send event.")
				c.sendMessage("Failed to confirm command - internal error. Check logs for details.")
				return
			}
			c.sendMessage(
				fmt.Sprintf("Confirmed and executing command: %v with values %v",
					state.chatToConfirm.Command.Name, state.chatToConfirm.Values))
			return
		}
		if Abort(message) {
			c.sendMessage(
				fmt.Sprintf("Aborted command: %v with values %v",
					state.chatToConfirm.Command.Name, state.chatToConfirm.Values))
			c.setState(convoState{
				phase: phaseInit,
			})
			return
		}
		c.sendMessage(state.chatToConfirm.Command.ConfirmationMessage(state.chatToConfirm.Values))
		return
	case phaseConfirmed:
		c.sendMessage(
			fmt.Sprintf("Already executing command: %v with values %v",
				state.confirmedChat.Command.Name, state.confirmedChat.Values))
	}
}

func (c *conversation) sendEvent(values map[string]interface{}) error {
	eventStruct, err := generateValues(c.bot.dialog.Commands[0], c.bot.valuesType, values)
	if err != nil {
		return trace.Wrap(err)
	}
	event := &ChatEvent{
		created: time.Now().UTC(),
		Values:  eventStruct,
		convo:   c,
	}
	select {
	case c.bot.eventsC <- event:
		return nil
	case <-c.ctx.Done():
		return trace.ConnectionProblem(nil, "context is closing")
	default:
		return trace.ConnectionProblem(nil, "queue is full")
	}
}

func (c *conversation) sendMessage(message string) error {
	return c.bot.sendMessage(c.ctx, c.channel, c.threadTimestamp, message)
}

type botConfig struct {
	valuesType reflect.Type
	client     *slack.Client
	parser     *ChatParser
	eventsC    chan force.Event
	log        force.Logger
	dialog     Dialog
}

type bot struct {
	botConfig
	convos map[string]*conversation
}

func newBot(cfg botConfig) *bot {
	return &bot{
		botConfig: cfg,
		convos:    make(map[string]*conversation),
	}
}

func (b *bot) isFromBot(info *slack.Info, event *slack.MessageEvent) bool {
	return len(event.User) == 0 || event.User == info.User.ID || len(event.BotID) > 0
}

func mention(userID string) string {
	return fmt.Sprintf("<@%v>", userID)
}

func (b *bot) isMentioned(info *slack.Info, text string) bool {
	return strings.Contains(text, mention(info.User.ID))
}

func (b *bot) sendMessage(ctx context.Context, channel, threadTimestamp, text string) error {
	_, _, err := b.client.PostMessage(channel,
		slack.MsgOptionText(text, false),
		slack.MsgOptionTS(threadTimestamp))
	return trace.Wrap(err)
}

func (b *bot) asyncSendMessage(ctx context.Context, channel, threadTimestamp, text string) {
	go func() {
		err := b.sendMessage(ctx, channel, threadTimestamp, text)
		if err != nil {
			b.log.WithError(err).Warningf("Failed to post message.")
		}
	}()
}

func (b *bot) Listen(ctx context.Context) error {
	rtm := b.client.NewRTM()
	go rtm.ManageConnection()

	for msg := range rtm.IncomingEvents {
		switch event := msg.Data.(type) {
		case *slack.MessageEvent:
			info := rtm.GetInfo()
			if b.isFromBot(info, event) {
				continue
			}
			// new mention
			threadTimestamp := event.ThreadTimestamp
			if threadTimestamp == "" {
				if b.isMentioned(info, event.Text) {
					// starting a new conversation
					threadTimestamp = event.Timestamp
					convo := conversation{
						ctx:  ctx,
						lock: &sync.RWMutex{},
						state: convoState{
							phase: phaseInit,
						},
						threadTimestamp: threadTimestamp,
						bot:             b,
						channel:         event.Channel,
						userID:          info.User.ID,
					}
					b.convos[convo.threadTimestamp] = &convo
					go convo.handleMessage(event.Text)
				}
			} else {
				// talking about existing thread
				convo, ok := b.convos[event.ThreadTimestamp]
				if ok {
					go convo.handleMessage(event.Text)
				} else {
					// respond that it does not recognize this conversation
					b.asyncSendMessage(ctx, event.Channel, event.ThreadTimestamp, unrecognizedConvoMessage)
				}
			}
		case *slack.PresenceChangeEvent:
		case *slack.LatencyReport:
		case *slack.RTMError:
			return trace.ConnectionProblem(nil, event.Msg)
		case *slack.InvalidAuthEvent:
			return trace.ConnectionProblem(nil, "failed to authenticate")
		default:
		}
	}
	return nil
}
