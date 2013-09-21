package main

import (
	"fmt"
	"log"
)

const (
	// Digilent Nexus2 board has 8MB of RAM.
	// 64 bit CPU would see this as 1MW, hence
	// 1048576.
	MEMWORDS = 1048576

	// Parameter stack depth, expressed as a mask, NOT including T.
	PSDEPTH = 16
	PSMASK  = PSDEPTH - 1

	// Return stack depth, expressed as a mask, NOT including R.
	RSDEPTH = 16
	RSMASK  = RSDEPTH - 1

	// Program control flow operand mask.
	operandMask = 0x003FFFFFFFFFFFFF
)

const (
	NOP = iota
	BRA
	BZ
	BC
	CALL
	RFS
	NEXT
	TIMES
	LDRP
	LDXP
	LDI
	LDX
	OR
	STXP
	RR8
	STX
	COM
	SHL
	SHR
	MULS
	XOR
	AND
	DIVS
	ADD
	POP
	TX
	DUP
	OVER
	PUSH
	XT
	__reserved_for_prefixed_opcodes__
	DROP
)

var (
	shifts = []uint{
		0, // dummy
		58,
		52,
		46,
		40,
		34,
		28,
		22,
		16,
		10,
		4,
	}
)

type VP64 struct {
	exitRequested bool
	memory        [MEMWORDS]uint64

	x      uint64
	t      uint64
	s      [PSDEPTH]uint64
	r      uint64
	rs     [RSDEPTH]uint64
	sp, rp int
	pc     uint64
	ip     uint64
	ir     uint64
	state  int
}

func (v *VP64) Reset() {
	v.pc = 0
	v.state = 0
	v.ir = 0
	v.sp = v.sp & 0xF
	v.rp = v.rp & 0xF
}

func (v *VP64) NextWord() uint64 {
	if v.pc >= MEMWORDS {
		log.Printf("Attempt to execute instruction at location $%X", v.pc)
		v.pc = v.pc % MEMWORDS
		log.Printf("Wrapping PC to $%X", v.pc)
	}
	x := v.memory[v.pc]
	v.pc++
	return x
}

func (v *VP64) getWord(a uint64) uint64 {
	if a >= MEMWORDS {
		log.Printf("Attempt to fetch word from $%X at $%X", a, v.ip)
		a = a % MEMWORDS
		log.Printf("Wrapping address to $%X", a)
	}
	return v.memory[a]
}

func (v *VP64) putWord(a, d uint64) {
	if a >= MEMWORDS {
		log.Printf("Attempt to store word $%X to address $%X, at $%X", d, a, v.ip)
		a = a % MEMWORDS
		log.Printf("Wrapping address to $%X", a)
		log.Printf("WARNING WARNING -- THIS ALMOST CERTAINLY CORRUPTS MEMORY")
	}
	v.memory[a] = d
}

func (v *VP64) Fetch() {
	v.ip = v.pc
	v.ir = v.NextWord()
	v.state = 1
}

func (v *VP64) operand() uint64 {
	x := (v.ir >> 4) & operandMask
	// sign extend
	if (x & 0x0020000000000000) != 0 {
		x = x | 0xFFC0000000000000
	}
	return x
}

func (v *VP64) branch(displacement uint64) {
	v.pc = v.pc + displacement
	v.Fetch()
}

func (v *VP64) pushR(datum uint64) {
	v.rs[v.rp] = v.r
	v.rp = (v.rp + 1) & RSMASK
	v.r = datum
}

func (v *VP64) popR() uint64 {
	x := v.r
	v.rp = (v.rp - 1) & RSMASK
	v.r = v.rs[v.rp]
	return x
}

func (v *VP64) pushS(datum uint64) {
	v.s[v.sp] = datum
	v.sp = (v.sp + 1) & PSMASK
}

func (v *VP64) popS() uint64 {
	v.sp = (v.sp - 1) & PSMASK
	return v.s[v.sp]
}

func (v *VP64) Execute() {
	if (v.state < 1) || (v.state > 10) {
		v.Fetch()
	}
	opcode := (v.ir >> shifts[v.state]) & 63
	v.state++
	switch opcode {
	case NOP:
		// No operation, by definition.

	case BRA:
		v.branch(v.operand())

	case BZ:
		if v.t == 0 {
			v.branch(v.operand())
		}

	case BC:
		if (v.t & 0x8000000000000000) != 0 {
			v.branch(v.operand())
		}

	case CALL:
		v.pushR(v.pc)
		v.branch(v.operand())

	case RFS:
		v.pc = v.popR()
		v.Fetch()

	case NEXT:
		if v.r != 0 {
			v.r--
			v.branch(v.operand())
		}

	case TIMES:
		if v.r != 0 {
			v.r--
			v.pc = v.ip
			v.Fetch()
		}

	case LDRP:
		v.pushS(v.t)
		v.t = v.getWord(v.r)
		v.r++

	case LDXP:
		v.pushS(v.t)
		v.t = v.getWord(v.x)
		v.x++

	case LDI:
		v.pushS(v.t)
		v.t = v.NextWord()

	case LDX:
		v.pushS(v.t)
		v.t = v.getWord(v.x)

	case OR:
		v.t = v.t | v.popS()

	case STXP:
		v.putWord(v.x, v.t)
		v.t = v.popS()
		v.x++

	case RR8:
		v.t = (v.t >> 8) | (v.t << 56)

	case STX:
		v.putWord(v.x, v.t)
		v.t = v.popS()

	case COM:
		v.t = ^v.t

	case SHL:
		v.t = v.t << 1

	case SHR:
		v.t = v.t >> 1

	case MULS:
		log.Printf("MULS not yet implemented")
		v.t = 0xDEADFEEDC0DE0BAD

	case XOR:
		v.t = v.t ^ v.popS()

	case AND:
		v.t = v.t & v.popS()

	case DIVS:
		log.Printf("DIVS not yet implemented")
		v.t = 0xFEEDDEADC0DE0BAD

	case ADD:
		v.t = v.t + v.popS()

	case POP:
		v.pushS(v.t)
		v.t = v.popR()

	case TX:
		v.pushS(v.t)
		v.t = v.x

	case DUP:
		v.pushS(v.t)

	case OVER:
		{
			x := v.s[v.sp]
			v.pushS(v.t)
			v.t = x
		}

	case PUSH:
		v.pushR(v.t)
		v.t = v.popS()

	case XT:
		v.x = v.t
		v.t = v.popS()

	case DROP:
		v.t = v.popS()

	default:
		log.Printf("Attempt to execute opcode %d at $%X", opcode, v.ip)
	}
}

func (v *VP64) LoadROMs() {
	v.memory[00] = 0x10000000000000F0
	v.memory[01] = 0x07FFFFFFFFFFFFE0
	v.memory[16] = 0x10000000000000F0
	v.memory[17] = 0xF801400000000000
	v.memory[32] = 0xFC029C1400000000
	v.memory[33] = 0x0000000000000000
}

func main() {
	fmt.Println("VP64 Emulator.")
	if len(shifts) != 11 {
		panic("Programmer error: len(shifts) != 11")
	}

	v := &VP64{}
	v.LoadROMs()

	for v.Execute(); !v.exitRequested; v.Execute() {
		// 1 clock cycle elapsed.
		// Emulate other hardware features here,
		// like SPI peripherals and stuff.
	}
}
