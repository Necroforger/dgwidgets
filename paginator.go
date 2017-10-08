package dgwidgets

import (
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/bwmarrin/discordgo"
)

// Paginator provides a method for creating a navigatable embed
type Paginator struct {
	sync.Mutex
	Pages []*discordgo.MessageEmbed
	Index int

	// Loop back to the beginning or end when on the first or last page.
	Loop   bool
	Widget *Widget

	Ses *discordgo.Session

	DeleteMessageWhenDone   bool
	DeleteReactionsWhenDone bool
	ColourWhenDone          int

	lockToUser bool

	running bool
}

// NewPaginator returns a new Paginator
//    ses      : discordgo session
//    channelID: channelID to spawn the paginator on
func NewPaginator(ses *discordgo.Session, channelID string) *Paginator {
	p := &Paginator{
		Ses:   ses,
		Pages: []*discordgo.MessageEmbed{},
		Index: 0,
		Loop:  false,
		DeleteMessageWhenDone:   false,
		DeleteReactionsWhenDone: false,
		ColourWhenDone:          -1,
		Widget:                  NewWidget(ses, channelID, nil),
	}
	p.addHandlers()

	return p
}

func (p *Paginator) addHandlers() {
	p.Widget.Handle(NavBeginning, func(w *Widget, r *discordgo.MessageReaction) {
		if err := p.Goto(0); err == nil {
			p.Update()
		}
	})
	p.Widget.Handle(NavLeft, func(w *Widget, r *discordgo.MessageReaction) {
		if err := p.PreviousPage(); err == nil {
			p.Update()
		}
	})
	p.Widget.Handle(NavRight, func(w *Widget, r *discordgo.MessageReaction) {
		if err := p.NextPage(); err == nil {
			p.Update()
		}
	})
	p.Widget.Handle(NavEnd, func(w *Widget, r *discordgo.MessageReaction) {
		if err := p.Goto(len(p.Pages) - 1); err == nil {
			p.Update()
		}
	})
	p.Widget.Handle(NavNumbers, func(w *Widget, r *discordgo.MessageReaction) {
		if msg, err := w.QueryInput("enter the page number you would like to open", r.UserID, 10*time.Second); err == nil {
			if n, err := strconv.Atoi(msg.Content); err == nil {
				p.Goto(n - 1)
				p.Update()
			}
		}
	})
}

// LockToUser causes the widget to ignore reactions added by people
// that didn't spawn the widget
func (p *Paginator) LockToUser(id string) error {
	if !userIDRegex.MatchString(id) {
		return ErrInvalidID
	}

	p.Widget.LockToUser = true
	p.Widget.Spawner = id

	return nil
}

// UnlockFromUser opens the widget to be affected by
// reactions from others
func (p *Paginator) UnlockFromUser() {
	p.Widget.LockToUser = false
	p.Widget.Spawner = ""
}

// Spawn spawns the paginator in channel p.ChannelID
func (p *Paginator) Spawn() error {
	if p.Running() {
		return ErrAlreadyRunning
	}
	p.Lock()
	p.running = true
	p.Unlock()

	defer func() {
		p.Lock()
		p.running = false
		p.Unlock()

		// Delete Message when done
		if p.DeleteMessageWhenDone && p.Widget.Message != nil {
			p.Ses.ChannelMessageDelete(p.Widget.Message.ChannelID, p.Widget.Message.ID)
		} else if p.ColourWhenDone >= 0 {
			if page, err := p.Page(); err == nil {
				page.Color = p.ColourWhenDone
				p.Update()
			}
		}

		// Delete reactions when done
		if p.DeleteReactionsWhenDone && p.Widget.Message != nil {
			p.Ses.MessageReactionsRemoveAll(p.Widget.ChannelID, p.Widget.Message.ID)
		}
	}()

	page, err := p.Page()
	if err != nil {
		return err
	}
	p.Widget.Embed = page

	return p.Widget.Spawn()
}

// Add a page to the paginator
//    embed: embed page to add.
func (p *Paginator) Add(embeds ...*discordgo.MessageEmbed) {
	p.Pages = append(p.Pages, embeds...)
}

// Page returns the page of the current index
func (p *Paginator) Page() (*discordgo.MessageEmbed, error) {
	p.Lock()
	defer p.Unlock()

	if p.Index < 0 || p.Index >= len(p.Pages) {
		return nil, ErrIndexOutOfBounds
	}

	return p.Pages[p.Index], nil
}

// NextPage sets the page index to the next page
func (p *Paginator) NextPage() error {
	p.Lock()
	defer p.Unlock()

	if p.Index+1 >= 0 && p.Index+1 < len(p.Pages) {
		p.Index++
		return nil
	}

	// Set the queue back to the beginning if Loop is enabled.
	if p.Loop {
		p.Index = 0
		return nil
	}

	return ErrIndexOutOfBounds
}

// PreviousPage sets the current page index to the previous page.
func (p *Paginator) PreviousPage() error {
	p.Lock()
	defer p.Unlock()

	if p.Index-1 >= 0 && p.Index-1 < len(p.Pages) {
		p.Index--
		return nil
	}

	// Set the queue back to the beginning if Loop is enabled.
	if p.Loop {
		p.Index = len(p.Pages) - 1
		return nil
	}

	return ErrIndexOutOfBounds
}

// Goto jumps to the requested page index
//    index: The index of the page to go to
func (p *Paginator) Goto(index int) error {
	p.Lock()
	defer p.Unlock()
	if index < 0 || index >= len(p.Pages) {
		return ErrIndexOutOfBounds
	}
	p.Index = index
	return nil
}

// Update updates the message with the current state of the paginator
func (p *Paginator) Update() error {
	if p.Widget.Message == nil {
		return ErrNilMessage
	}

	page, err := p.Page()
	if err != nil {
		return err
	}

	_, err = p.Widget.UpdateEmbed(page)
	return err
}

// Running returns the running status of the paginator
func (p *Paginator) Running() bool {
	p.Lock()
	running := p.running
	p.Unlock()
	return running
}

// SetPageFooters sets the footer of each embed to
// Be its page number out of the total length of the embeds.
func (p *Paginator) SetPageFooters() {
	for index, embed := range p.Pages {
		embed.Footer = &discordgo.MessageEmbedFooter{
			Text: fmt.Sprintf("#[%d / %d]", index+1, len(p.Pages)),
		}
	}
}
