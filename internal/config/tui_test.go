package config

import (
	"io"
	"testing"
	"time"

	"github.com/charmbracelet/bubbles/cursor"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
)

func TestModel_Init(t *testing.T) {
	type fields struct {
		fields []field
	}
	tests := []struct {
		name       string
		fields     fields
		assertions func(*assert.Assertions, tea.Cmd)
	}{
		{
			name: "success",
			fields: fields{
				fields: []field{
					{
						name:    "name",
						preview: "preview",
						input:   defaultTextInput(),
					},
				},
			},
			assertions: func(a *assert.Assertions, cmd tea.Cmd) {
				a.IsType(cursor.BlinkMsg{}, cmd())
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &Model{
				fields: tt.fields.fields,
			}
			cmd := m.Init()
			tt.assertions(assert.New(t), cmd)
		})
	}
}

func TestModel_Update(t *testing.T) {
	type fields struct {
		cursor int
		fields []field
		conf   map[string]interface{}
	}
	type args struct {
		msg tea.Msg
	}
	tests := []struct {
		name       string
		fields     fields
		args       args
		assertions func(*assert.Assertions, tea.Model, tea.Cmd)
	}{
		{
			name: "escape button pressed",
			fields: fields{
				cursor: 0,
				fields: []field{
					{
						input: defaultTextInput(),
					},
				},
			},
			args: args{
				msg: tea.KeyMsg{
					Type: tea.KeyEsc,
				},
			},
			assertions: func(a *assert.Assertions, model tea.Model, cmd tea.Cmd) {
				a.Equal(ErrUserAborted, model.(*Model).err)
				a.IsType(tea.QuitMsg{}, cmd())
			},
		},
		{
			name: "enter button pressed",
			fields: fields{
				cursor: 0,
				fields: []field{
					{
						name: "field1",
						input: func() textinput.Model {
							m := defaultTextInput()
							m.Focus()
							m.SetValue("new value")
							return m
						}(),
					},
					{
						name:  "field2",
						input: defaultTextInput(),
					},
				},
				conf: map[string]interface{}{
					"field1": "value1",
					"field2": "value2",
				},
			},
			args: args{
				msg: tea.KeyMsg{
					Type: tea.KeyEnter,
				},
			},
			assertions: func(a *assert.Assertions, model tea.Model, cmd tea.Cmd) {
				m := model.(*Model)
				a.Nil(m.err)
				a.Equal(m.conf["field1"], "new value")
				a.Equal(1, m.cursor)
				a.IsType(cursor.BlinkMsg{}, cmd())
			},
		},
		{
			name: "enter button pressed with last field",
			fields: fields{
				cursor: 0,
				fields: []field{
					{
						name: "field1",
						input: func() textinput.Model {
							m := defaultTextInput()
							m.Focus()
							m.SetValue("new value")
							return m
						}(),
					},
				},
				conf: map[string]interface{}{
					"field1": "value1",
				},
			},
			args: args{
				msg: tea.KeyMsg{
					Type: tea.KeyEnter,
				},
			},
			assertions: func(a *assert.Assertions, model tea.Model, cmd tea.Cmd) {
				m := model.(*Model)
				a.Nil(m.err)
				a.Equal(m.conf["field1"], "new value")
				a.Equal(0, m.cursor)
				a.IsType(tea.QuitMsg{}, cmd())
			},
		},
		{
			name: "tab button pressed",
			fields: fields{
				cursor: 0,
				fields: []field{
					{
						name:    "field1",
						preview: "preview1",
						input: func() textinput.Model {
							m := defaultTextInput()
							m.Focus()
							m.Placeholder = "preview1"
							return m
						}(),
					},
				},
				conf: map[string]interface{}{
					"field1": "value1",
				},
			},
			args: args{
				msg: tea.KeyMsg{
					Type: tea.KeyTab,
				},
			},
			assertions: func(a *assert.Assertions, model tea.Model, cmd tea.Cmd) {
				m := model.(*Model)
				a.Nil(m.err)
				a.Equal("preview1", m.fields[0].input.Value())
				a.Equal(0, m.cursor)
				a.Nil(cmd)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &Model{
				cursor: tt.fields.cursor,
				fields: tt.fields.fields,
				conf:   tt.fields.conf,
			}
			model, cmd := m.Update(tt.args.msg)
			tt.assertions(assert.New(t), model, cmd)
		})
	}
}

func TestModel_View(t *testing.T) {
	type fields struct {
		fields []field
	}
	tests := []struct {
		name       string
		fields     fields
		assertions func(*assert.Assertions, string)
	}{
		{
			name: "success with one field",
			fields: fields{
				fields: []field{
					{
						name: "field1",
						input: func() textinput.Model {
							m := defaultTextInput()
							m.Focus()
							m.SetValue("value1")
							return m
						}(),
					},
				},
			},
			assertions: func(a *assert.Assertions, view string) {
				a.Contains(view, "> field1: value1")
				a.Contains(view, helpMessage)
			},
		},
		{
			name: "success with two fields",
			fields: fields{
				fields: []field{
					{
						name: "field1",
						input: func() textinput.Model {
							m := defaultTextInput()
							m.SetValue("value1")
							return m
						}(),
					},
					{
						name: "field2",
						input: func() textinput.Model {
							m := defaultTextInput()
							m.Focus()
							m.SetValue("value2")
							return m
						}(),
					},
				},
			},
			assertions: func(a *assert.Assertions, view string) {
				a.Contains(view, "field1: value1")
				a.Contains(view, "> field2: value2")
				a.Contains(view, helpMessage)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &Model{
				fields: tt.fields.fields,
			}
			tt.assertions(assert.New(t), m.View())
		})
	}
}

func TestModel_updateConfigWithFieldInput(t *testing.T) {
	type fields struct {
		conf map[string]interface{}
	}
	type args struct {
		f *field
	}
	tests := []struct {
		name       string
		fields     fields
		args       args
		assertions func(*assert.Assertions, *Model)
	}{
		{
			name: "field type slice",
			fields: fields{
				conf: map[string]interface{}{
					"field1": make([]string, 0),
				},
			},
			args: args{
				f: &field{
					name: "field1",
					input: func() textinput.Model {
						m := defaultTextInput()
						m.SetValue("value1")
						return m
					}(),
				},
			},
			assertions: func(a *assert.Assertions, m *Model) {
				a.Nil(m.err)
				a.Equal([]string{"value1"}, m.conf["field1"])
			},
		},
		{
			name: "field type slice empty",
			fields: fields{
				conf: map[string]interface{}{
					"field1": make([]string, 0),
				},
			},
			args: args{
				f: &field{
					name: "field1",
					input: func() textinput.Model {
						m := defaultTextInput()
						m.SetValue("")
						return m
					}(),
				},
			},
			assertions: func(a *assert.Assertions, m *Model) {
				a.Nil(m.err)
				a.Equal([]string{}, m.conf["field1"])
			},
		},
		{
			name: "field type bool",
			fields: fields{
				conf: map[string]interface{}{
					"field1": true,
				},
			},
			args: args{
				f: &field{
					name: "field1",
					input: func() textinput.Model {
						m := defaultTextInput()
						m.SetValue("false")
						return m
					}(),
				},
			},
			assertions: func(a *assert.Assertions, m *Model) {
				a.Nil(m.err)
				a.Equal(false, m.conf["field1"])
			},
		},
		{
			name: "field type bool error",
			fields: fields{
				conf: map[string]interface{}{
					"field1": true,
				},
			},
			args: args{
				f: &field{
					name: "field1",
					input: func() textinput.Model {
						m := defaultTextInput()
						m.SetValue("invalid")
						return m
					}(),
				},
			},
			assertions: func(a *assert.Assertions, m *Model) {
				a.NotNil(m.err)
			},
		},
		{
			name: "field type string",
			fields: fields{
				conf: map[string]interface{}{
					"field1": "value1",
				},
			},
			args: args{
				f: &field{
					name: "field1",
					input: func() textinput.Model {
						m := defaultTextInput()
						m.SetValue("value1")
						return m
					}(),
				},
			},
			assertions: func(a *assert.Assertions, m *Model) {
				a.Nil(m.err)
				a.Equal("value1", m.conf["field1"])
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &Model{
				conf: tt.fields.conf,
			}
			m.updateConfigWithFieldInput(tt.args.f)
			tt.assertions(assert.New(t), m)
		})
	}
}

func TestNewTeaProgram(t *testing.T) {
	type args struct {
		conf map[string]interface{}
	}
	tests := []struct {
		name       string
		args       args
		assertions func(*assert.Assertions, *tea.Program)
	}{
		{
			name: "success",
			args: args{
				conf: map[string]interface{}{
					"field3": []string{
						"value3.1",
						"value3.2",
					},
					"field1": "value1",
					"field2": "value2",
				},
			},
			assertions: func(a *assert.Assertions, p *tea.Program) {
				a.NotNil(p)
				go func() {
					time.Sleep(1 * time.Second)
					p.Quit()
				}()
				model, err := p.Run()
				a.Nil(err)
				m, ok := model.(*Model)
				a.True(ok)
				a.Equal("value1", m.conf["field1"])
				a.Equal("value2", m.conf["field2"])
				a.Equal([]string{
					"value3.1",
					"value3.2",
				}, m.conf["field3"])
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.assertions(assert.New(t), NewTeaProgram(tt.args.conf, tea.WithInput(nil), tea.WithOutput(io.Discard)))
		})
	}
}
