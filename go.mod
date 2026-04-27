module github.com/carledwards/6502-sim-tui

go 1.26.2

replace github.com/carledwards/foxpro-go => ../foxpro-go

replace github.com/carledwards/6502-netsim-go => ../6502-netsim-go

require (
	github.com/carledwards/6502-netsim-go v0.0.0-00010101000000-000000000000
	github.com/carledwards/foxpro-go v0.0.0-00010101000000-000000000000
	github.com/gdamore/tcell/v2 v2.7.4
)

require (
	github.com/gdamore/encoding v1.0.0 // indirect
	github.com/lucasb-eyer/go-colorful v1.2.0 // indirect
	github.com/mattn/go-runewidth v0.0.15 // indirect
	github.com/rivo/uniseg v0.4.3 // indirect
	golang.org/x/sys v0.17.0 // indirect
	golang.org/x/term v0.17.0 // indirect
	golang.org/x/text v0.14.0 // indirect
)
