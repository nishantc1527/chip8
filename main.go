package main

import (
	"errors"
	"fmt"
	"github.com/akamensky/argparse"
	"github.com/veandco/go-sdl2/mix"
	"github.com/veandco/go-sdl2/sdl"
	"log"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"time"
)

var (
	memory [0x1000]uint8

	v        [0x10]uint8 // VX Registers
	iAddress uint16      // I Register
	dt       uint8       // Delay Timer
	st       uint8       // Sound Timer
	pc       uint16      // Program Counter
	sp       uint8       // Stack Pointer
	stack    [0x10]uint16

	instruction uint16

	oldPixels [0x20][0x40]bool
	pixels    [0x20][0x40]bool

	keysPressed [0x10]bool
	keymap      [0x10]uint8

	beep *mix.Chunk

	na bool
)

const (
	scaleValue = 10
)

func main() {
	initialize()
	mainLoop()
}

func initialize() {
	parser := argparse.NewParser("Chip 8", "A Chip 8 Emulator")

	listGames := parser.Flag("l", "list", &argparse.Options{Required: false, Help: "List All Inbuilt Games", Default: false})
	game := parser.String("g", "game", &argparse.Options{Required: false, Help: "Inbuilt Game To Play", Default: "pong2"})
	rom := parser.String("r", "rom", &argparse.Options{Required: false, Help: "Path To Rom File"})
	audio := parser.Flag("a", "no-audio", &argparse.Options{Required: false, Help: "If True, Beeping Sounds Will Be Played", Default: false})

	err := parser.Parse(os.Args)
	if err != nil {
		fmt.Print(parser.Usage(err))
		os.Exit(1)
	}

	if *listGames {
		files, _ := AssetDir("assets")

		for _, file := range files {
			if file == "beep.wav" {
				continue
			}

			fmt.Println(strings.TrimSuffix(file, filepath.Ext(file)))
		}

		os.Exit(0)
	}

	bytes, readErr := os.ReadFile(*rom)
	if readErr == nil {
		if len(bytes) > 0x4096 {
			panic(errors.New("input rom to large"))
		}
		loadRom(bytes)
	} else {
		bytes, _ = Asset("assets/" + *game + ".rom")
		loadRom(bytes)
	}

	if *audio {
		na = true
	}

	pc = 0x200

	sprites := GetSprites()
	loadSprites(sprites)

	keymap = GetKeymap()
}

func loadSprites(sprites [0x50]byte) {
	for i := 0; i < len(sprites); i++ {
		memory[i] = sprites[i]
	}
}

func loadRom(bytes []byte) {
	for i, j := 0, 0x200; i < len(bytes); i, j = i+1, j+1 {
		memory[j] = bytes[i]
	}
}

func mainLoop() {
	if err := sdl.Init(sdl.INIT_EVERYTHING); err != nil {
		panic(err)
	}
	defer sdl.Quit()

	window, err := sdl.CreateWindow("Chip 8", sdl.WINDOWPOS_UNDEFINED, sdl.WINDOWPOS_UNDEFINED, 64*scaleValue, 32*scaleValue, sdl.WINDOW_SHOWN)
	if err != nil {
		panic(err)
	}
	defer window.Destroy()

	renderer, err := sdl.CreateRenderer(window, -1, sdl.RENDERER_ACCELERATED)
	if err != nil {
		panic(err)
	}
	defer renderer.Destroy()

	if err := mix.Init(mix.INIT_MP3); err != nil {
		panic(err)
	}
	defer mix.Quit()

	if err := mix.OpenAudio(22050, mix.DEFAULT_FORMAT, 2, 4096); err != nil {
		log.Println(err)
		return
	}
	defer mix.CloseAudio()

	data, err := Asset("assets/beep.wav")
	if err != nil {
		log.Println(err)
	}

	chunk, err := mix.QuickLoadWAV(data)
	if err != nil {
		log.Println(err)
	}
	defer chunk.Free()

	beep = chunk

	for {
		displayGraphics(renderer)
		pollInput()
		clock()
		sleep()
	}
}

func displayGraphics(renderer *sdl.Renderer) {
	if pixelsChanged() {
		oldPixels = pixels
		renderer.SetDrawColor(0, 0, 0, 255)
		renderer.Clear()

		for i := 0; i < 0x20; i++ {
			for j := 0; j < 0x40; j++ {
				if !pixels[i][j] {
					renderer.SetDrawColor(0, 0, 0, 255)
				} else {
					renderer.SetDrawColor(255, 255, 255, 255)
				}

				renderer.FillRect(&sdl.Rect{
					X: int32(j * scaleValue),
					Y: int32(i * scaleValue),
					W: scaleValue,
					H: scaleValue,
				})
			}
		}

		renderer.Present()
	}
}

func pixelsChanged() bool {
	return oldPixels != pixels
}

func pollInput() {
	for event := sdl.PollEvent(); event != nil; event = sdl.PollEvent() {
		switch t := event.(type) {
		case *sdl.QuitEvent:
			os.Exit(0)
		case *sdl.KeyboardEvent:
			switch t.GetType() {
			case sdl.KEYDOWN:
				for i := 0; i < 16; i++ {
					if t.Keysym.Sym == sdl.Keycode(keymap[i]) {
						keysPressed[i] = true
					}
				}
			case sdl.KEYUP:
				for i := 0; i < 16; i++ {
					if t.Keysym.Sym == sdl.Keycode(keymap[i]) {
						keysPressed[i] = false
					}
				}
			}
		}
	}
}

func clock() {
	instruction = (uint16(memory[pc]) << 8) | uint16(memory[pc+1])

	switch instruction >> 12 {
	case 0x0:
		switch getKK() {
		case 0xe0:
			for i := 0; i < 0x20; i++ {
				for j := 0; j < 0x40; j++ {
					pixels[i][j] = false
				}
			}
		case 0xee:
			sp--
			pc = stack[sp]
		}
	case 0x1:
		pc = getAddr()
		sameInstruction()
	case 0x2:
		stack[sp] = pc
		sp++
		pc = getAddr()
		sameInstruction()
	case 0x3:
		if vx() == getKK() {
			nextInstruction()
		}
	case 0x4:
		if vx() != getKK() {
			nextInstruction()
		}
	case 0x5:
		if vx() == vy() {
			nextInstruction()
		}
	case 0x6:
		v[getX()] = getKK()
	case 0x7:
		v[getX()] += getKK()
	case 0x8:
		switch getNibble() {
		case 0:
			v[getX()] = vy()
		case 1:
			v[getX()] |= vy()
		case 2:
			v[getX()] &= vy()
		case 3:
			v[getX()] ^= vy()
		case 4:
			sum := uint16(vx()) + uint16(vy())

			if (sum & 0xff00) > 0 {
				v[0xf] = 1
			} else {
				v[0xf] = 0
			}

			v[getX()] = uint8(sum & 0x00ff)
		case 5:
			difference := int8(vx()) - int8(vy())

			if (uint8(difference) & uint8(1<<7)) > 0 {
				v[0xf] = 0
			} else {
				v[0xf] = 1
			}

			v[getX()] = uint8(difference)
		case 6:
			if (vx() & 1) > 0 {
				v[0xf] = 1
			} else {
				v[0xf] = 0
			}

			v[getX()] = vx() >> 1
		case 7:
			difference := int8(vy()) - int8(vx())

			if (uint8(difference) & uint8(1<<7)) > 0 {
				v[0xf] = 0
			} else {
				v[0xf] = 1
			}

			v[getX()] = uint8(difference)
		case 0xe:
			if (vx() & (1 << 7)) > 0 {
				v[0xf] = 1
			} else {
				v[0xf] = 0
			}

			v[getX()] = vx() << 1
		}
	case 0x9:
		if vx() != vy() {
			nextInstruction()
		}
	case 0xa:
		iAddress = getAddr()
	case 0xb:
		pc = getAddr() + uint16(v[0x0])
	case 0xc:
		v[getX()] = uint8(rand.Int()) & getKK() // TODO Need to generate seed
	case 0xd:
		v[0xf] = 0
		bytes := make([]uint8, getNibble())

		for i := 0; i < len(bytes); i++ {
			bytes[i] = memory[uint16(i)+iAddress]
		}

		x, y := vx(), vy()
		initialX := x
		x %= 64

		for _, sprite := range bytes {
			currentBit := 1 << 7
			y %= 32

			for currentBit > 0 {
				x %= 64
				if (sprite & uint8(currentBit)) > 0 {
					if pixels[y][x] {
						pixels[y][x] = false
						v[0xf] = 1
					} else {
						pixels[y][x] = true
					}
				}

				currentBit >>= 1
				x++
			}

			x, y = initialX, y+1
		}
	case 0xe:
		switch getKK() {
		case 0x9e:
			if keysPressed[vx()] {
				nextInstruction()
			}
		case 0xa1:
			if !keysPressed[vx()] {
				nextInstruction()
			}
		}
	case 0xf:
		switch getKK() {
		case 0x07:
			v[getX()] = dt
		case 0x0a:
			// TODO Implement
			// All execution stops until a key is pressed, then the value of that key is stored in Vx.
		case 0x15:
			dt = vx()
		case 0x18:
			st = vx()
		case 0x1e:
			iAddress += uint16(vx())
		case 0x29:
			x := vx()
			iAddress = uint16(x * 5)
		case 0x33:
			ones := (vx() / 1) % 10
			tens := (vx() / 10) % 10
			hundreds := (vx() / 100) % 10

			memory[iAddress] = hundreds
			memory[iAddress+1] = tens
			memory[iAddress+2] = ones
		case 0x55:
			x := getX()
			for i := 0; i <= int(x); i++ {
				memory[uint16(i)+iAddress] = v[i]
			}
		case 0x65:
			x := getX()
			for i := 0; i <= int(x); i++ {
				v[i] = memory[uint16(i)+iAddress]
			}
		}
	}

	nextInstruction()

	if st > 0 && !na {
		if _, err := beep.Play(1, 0); err != nil {
			panic(err)
		}
	}

	if dt > 0 {
		dt--
	}

	if st > 0 {
		st--
	}
}

func sameInstruction() {
	pc -= 2
}

func nextInstruction() {
	pc += 2
}

func vx() uint8 {
	return v[getX()]
}

func vy() uint8 {
	return v[getY()]
}

func getAddr() uint16 {
	return (instruction & 0x0fff) >> 0b0
}

func getNibble() uint8 /* Technically a 4 bit value */ {
	return uint8((instruction & 0x000f) >> 0b0)
}

func getX() uint8 /* Technically a 4 bit value */ {
	return uint8((instruction & 0x0f00) >> 0b1000)
}

func getY() uint8 /* Technically a 4 bit value */ {
	return uint8((instruction & 0x00f0) >> 0b100)
}

func getKK() uint8 {
	return uint8((instruction & 0x00ff) >> 0b0)
}

func sleep() {
	time.Sleep(1 * time.Millisecond)
}

func GetSprites() [0x50]uint8 {
	return [0x50]byte{
		0b11110000,
		0b10010000,
		0b10010000,
		0b10010000,
		0b11110000,

		0b00100000,
		0b01100000,
		0b00100000,
		0b00100000,
		0b01110000,

		0b11110000,
		0b00010000,
		0b11110000,
		0b10000000,
		0b11110000,

		0b11110000,
		0b00010000,
		0b11110000,
		0b00010000,
		0b11110000,

		0b10010000,
		0b10010000,
		0b11110000,
		0b00010000,
		0b00010000,

		0b11110000,
		0b10000000,
		0b11110000,
		0b00010000,
		0b11110000,

		0b11110000,
		0b10000000,
		0b11110000,
		0b10010000,
		0b11110000,

		0b11110000,
		0b00010000,
		0b00100000,
		0b01000000,
		0b01000000,

		0b11110000,
		0b10010000,
		0b11110000,
		0b10010000,
		0b11110000,

		0b11110000,
		0b10010000,
		0b11110000,
		0b00010000,
		0b11110000,

		0b11110000,
		0b10010000,
		0b11110000,
		0b10010000,
		0b10010000,

		0b11100000,
		0b10010000,
		0b11100000,
		0b10010000,
		0b11100000,

		0b11110000,
		0b10000000,
		0b10000000,
		0b10000000,
		0b11110000,

		0b11100000,
		0b10010000,
		0b10010000,
		0b10010000,
		0b11100000,

		0b11110000,
		0b10000000,
		0b11110000,
		0b10000000,
		0b11110000,

		0b11110000,
		0b10000000,
		0b11110000,
		0b10000000,
		0b10000000,
	}
}

func GetKeymap() [16]uint8 {
	return [16]uint8{
		sdl.K_x,
		sdl.K_1,
		sdl.K_2,
		sdl.K_3,
		sdl.K_q,
		sdl.K_w,
		sdl.K_e,
		sdl.K_a,
		sdl.K_s,
		sdl.K_d,
		sdl.K_z,
		sdl.K_c,
		sdl.K_4,
		sdl.K_r,
		sdl.K_f,
		sdl.K_v,
	}
}
