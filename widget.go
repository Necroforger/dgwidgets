package dgwidgets

import (
	"errors"
	"sync"
	"time"

	"github.com/bwmarrin/discordgo"
)

// error vars
var (
	ErrAlreadyRunning   = errors.New("err: Widget already running")
	ErrIndexOutOfBounds = errors.New("err: Index is out of bounds")
	ErrNilMessage       = errors.New("err: Message is nil")
	ErrNilEmbed         = errors.New("err: embed is nil")
	ErrNotRunning       = errors.New("err: not running")
)

// WidgetHandler ...
type WidgetHandler func(*Widget, *discordgo.MessageReaction)

// Widget is a message embed with reactions for buttons.
// Accepts custom handlers for reactions.
type Widget struct {
	sync.Mutex
	Embed             *discordgo.MessageEmbed
	Message           *discordgo.Message
	Ses               *discordgo.Session
	ChannelID         string
	NavigationTimeout time.Duration
	Close             chan bool

	// Handlers binds emoji names to functions
	Handlers map[string]WidgetHandler
	// keys stores the handlers keys in the order they were added
	Keys []string

	running bool
}

// NewWidget returns a pointer to a Widget object
//    ses      : discordgo session
//    channelID: channelID to spawn the widget on
func NewWidget(ses *discordgo.Session, channelID string, embed *discordgo.MessageEmbed) *Widget {
	return &Widget{
		ChannelID: channelID,
		Ses:       ses,
		Keys:      []string{},
		Handlers:  map[string]WidgetHandler{},
		Close:     make(chan bool),
		Embed:     embed,
	}
}

// Spawn spawns the widget in channel w.ChannelID
func (w *Widget) Spawn() error {
	if w.Running() {
		return ErrAlreadyRunning
	}
	w.running = true
	defer func() {
		w.running = false
	}()

	if w.Embed == nil {
		return ErrNilEmbed
	}

	startTime := time.Now()

	// Create initial message.
	msg, err := w.Ses.ChannelMessageSendEmbed(w.ChannelID, w.Embed)
	if err != nil {
		return err
	}
	w.Message = msg

	// Add reaction buttons
	for _, v := range w.Keys {
		w.Ses.MessageReactionAdd(w.Message.ChannelID, w.Message.ID, v)
	}

	var reaction *discordgo.MessageReaction
	for {
		// Navigation timeout enabled
		if w.NavigationTimeout != 0 {
			select {
			case k := <-nextMessageReactionAddC(w.Ses):
				reaction = k.MessageReaction
			case <-time.After(startTime.Add(w.NavigationTimeout).Sub(time.Now())):
				return nil
			case <-w.Close:
				return nil
			}
		} else /*Navigation timeout not enabled*/ {
			select {
			case k := <-nextMessageReactionAddC(w.Ses):
				reaction = k.MessageReaction
			case <-w.Close:
				return nil
			}
		}

		// Ignore reactions sent by bot
		if reaction.MessageID != w.Message.ID || w.Ses.State.User.ID == reaction.UserID {
			continue
		}

		if v, ok := w.Handlers[reaction.Emoji.Name]; ok {
			v(w, reaction)
		}

		go func() {
			time.Sleep(time.Millisecond * 250)
			w.Ses.MessageReactionRemove(reaction.ChannelID, reaction.MessageID, reaction.Emoji.Name, reaction.UserID)
		}()
	}
}

// Handle adds a handler for the given emoji name
//    emojiName: The unicode value of the emoji
//    handler  : handler function to call when the emoji is clicked
//               func(*Widget, *discordgo.MessageReaction)
func (w *Widget) Handle(emojiName string, handler WidgetHandler) error {
	if _, ok := w.Handlers[emojiName]; !ok {
		w.Keys = append(w.Keys, emojiName)
		w.Handlers[emojiName] = handler
	}
	// if the widget is running, append the added emoji to the message.
	if w.Running() && w.Message != nil {
		return w.Ses.MessageReactionAdd(w.Message.ChannelID, w.Message.ID, emojiName)
	}
	return nil
}

// Running returns w.running
func (w *Widget) Running() bool {
	w.Lock()
	running := w.running
	w.Unlock()
	return running
}

// UpdateEmbed updates the embed object and edits the original message
//    embed: New embed object to replace w.Embed
func (w *Widget) UpdateEmbed(embed *discordgo.MessageEmbed) (*discordgo.Message, error) {
	if !w.Running() {
		return nil, ErrNotRunning
	}
	if w.Message == nil {
		return nil, ErrNilMessage
	}
	return w.Ses.ChannelMessageEditEmbed(w.ChannelID, w.Message.ID, embed)
}

// NextMessageReactionAddC returns a channel for the next MessageReactionAdd event
func nextMessageReactionAddC(s *discordgo.Session) chan *discordgo.MessageReactionAdd {
	out := make(chan *discordgo.MessageReactionAdd)
	s.AddHandlerOnce(func(_ *discordgo.Session, e *discordgo.MessageReactionAdd) {
		out <- e
	})
	return out
}
