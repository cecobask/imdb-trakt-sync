package config

import (
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type field struct {
	name    string
	preview string
	input   textinput.Model
}

func (f *field) activate() tea.Cmd {
	f.input.Cursor.Style = focusedStyle
	f.input.TextStyle = focusedStyle
	f.input.PromptStyle = focusedStyle
	f.input.Placeholder = f.preview
	return f.input.Focus()
}

func (f *field) deactivate() {
	f.input.Cursor.Style = noStyle
	f.input.TextStyle = noStyle
	f.input.PromptStyle = noStyle
	f.input.Placeholder = ""
	f.input.Blur()
}

type Model struct {
	cursor int
	fields []field
	conf   map[string]interface{}
	err    error
}

func (m *Model) Init() tea.Cmd {
	return m.fields[0].activate()
}

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch message := msg.(type) {
	case tea.KeyMsg:
		currentField := &m.fields[m.cursor]
		switch message.Type {
		case tea.KeyEsc, tea.KeyBreak:
			currentField.deactivate()
			m.err = ErrUserAborted
			return m, tea.Quit
		case tea.KeyEnter:
			currentField.deactivate()
			m.updateConfigWithFieldInput(currentField)
			if m.cursor == len(m.fields)-1 {
				return m, tea.Quit
			}
			m.cursor++
			nextField := &m.fields[m.cursor]
			return m, nextField.activate()
		case tea.KeyTab:
			currentField.input.SetValue(currentField.input.Placeholder)
		}
	}
	return m, m.updateInput(msg)
}

func (m *Model) View() string {
	var sb strings.Builder
	for _, f := range m.fields {
		if f.input.Focused() {
			sb.WriteString(focusedStyle.Render("> "))
		}
		sb.WriteString(f.name + ": " + f.input.View() + "\n")
	}
	sb.WriteString(helpStyle.Render(helpMessage))
	return sb.String()
}

func (m *Model) updateConfigWithFieldInput(f *field) {
	switch reflect.TypeOf(m.conf[f.name]).Kind() {
	case reflect.Slice:
		if f.input.Value() == "" {
			m.conf[f.name] = make([]string, 0)
			return
		}
		m.conf[f.name] = strings.Split(f.input.Value(), ",")
	case reflect.Bool:
		b, err := strconv.ParseBool(f.input.Value())
		if err != nil {
			m.err = fmt.Errorf("error parsing boolean: %w", err)
			return
		}
		m.conf[f.name] = b
	default:
		m.conf[f.name] = f.input.Value()
	}
}

func (m *Model) updateInput(msg tea.Msg) tea.Cmd {
	commands := make([]tea.Cmd, len(m.fields))
	for i := range m.fields {
		m.fields[i].input, commands[i] = m.fields[i].input.Update(msg)
	}
	return tea.Batch(commands...)
}

func (m *Model) Err() error {
	return m.err
}

func (m *Model) Config() map[string]interface{} {
	return m.conf
}

func NewTeaProgram(conf map[string]interface{}, opts ...tea.ProgramOption) *tea.Program {
	keys := make([]string, 0, len(conf))
	for k := range conf {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	m := Model{
		fields: make([]field, 0, len(conf)),
		conf:   conf,
	}
	for _, key := range keys {
		value := conf[key]
		if reflect.TypeOf(value).Kind() == reflect.Slice {
			value = strings.Trim(strings.Join(strings.Fields(fmt.Sprintf("%v", value)), ","), "[]")
		}
		m.fields = append(m.fields, field{
			name:    key,
			preview: fmt.Sprintf("%v", value),
			input:   defaultTextInput(),
		})
	}
	return tea.NewProgram(&m, opts...)
}

const helpMessage = "\n—— TAB autocomplete —— ENTER confirm —— ESC abort ——\n"

var (
	focusedStyle = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{
		Light: "#6200EE",
		Dark:  "#BB86FC",
	})
	noStyle        = lipgloss.NewStyle()
	helpStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	ErrUserAborted = errors.New("user aborted")
)

func defaultTextInput() textinput.Model {
	m := textinput.New()
	m.Prompt = ""
	return m
}
