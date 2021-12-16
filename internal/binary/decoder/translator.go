/*
 * Copyright 2021 ByteDance Inc.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package decoder

import (
    `fmt`

    `github.com/cloudwego/frugal/internal/atm`
    `github.com/cloudwego/frugal/internal/binary/defs`
    `github.com/cloudwego/frugal/internal/rt`
)

/** Function Prototype
 *
 *      func(buf unsafe.Pointer, nb int, i int, p unsafe.Pointer, rs *RuntimeState, st int) (pos int, err error)
 */

const (
    ARG_buf = 0
    ARG_nb  = 1
    ARG_i   = 2
    ARG_p   = 3
    ARG_rs  = 4
    ARG_st  = 5
)

const (
    RET_pos      = 0
    RET_err_itab = 1
    RET_err_data = 2
)

/** Register Allocations
 *
 *      P1      Current Working Pointer
 *      P2      Input Buffer Pointer
 *      P3      Runtime State Pointer
 *      P4      Error Type Pointer
 *      P5      Error Value Pointer
 *
 *      R2      Input Cursor
 *      R3      State Index
 *      R4      Field Tag
 */

const (
    WP = atm.P1
    IP = atm.P2
    RS = atm.P3
    ET = atm.P4     // may also be used as a temporary pointer register
    EP = atm.P5     // may also be used as a temporary pointer register
)

const (
    IC = atm.R2
    ST = atm.R3
    TG = atm.R4
)

const (
    TP = atm.P0
    TR = atm.R0
    UR = atm.R1
)

const (
    LB_eof      = "_eof"
    LB_halt     = "_halt"
    LB_type     = "_type"
    LB_skip     = "_skip"
    LB_error    = "_error"
    LB_missing  = "_missing"
    LB_overflow = "_overflow"
)

var (
    _E_overflow  error
    _V_zerovalue uint64
)

func init() {
    _E_overflow = fmt.Errorf("frugal: decoder stack overflow")
}

func Translate(s Program) atm.Program {
    p := atm.CreateBuilder()
    prologue (p)
    program  (p, s)
    epilogue (p)
    errors   (p)
    return p.Build()
}

func errors(p *atm.Builder) {
    p.Label (LB_eof)                    // _eof:
    p.LDAQ  (ARG_nb, UR)                // UR <= ARG.nb
    p.SUB   (TR, UR, TR)                // TR <= TR - UR
    p.GCALL (F_error_eof).              // GCALL error_eof:
      A0    (TR).                       //     n        <= TR
      R0    (ET).                       //     ret.itab => ET
      R1    (EP)                        //     ret.data => EP
    p.JAL   (LB_error, atm.Pn)          // GOTO _error
    p.Label (LB_type)                   // _type:
    p.GCALL (F_error_type).             // GCALL error_type:
      A0    (UR).                       //     e        <= UR
      A1    (TR).                       //     t        <= TR
      R0    (ET).                       //     ret.itab => ET
      R1    (EP)                        //     ret.data => EP
    p.JAL   (LB_error, atm.Pn)          // GOTO _error
    p.Label (LB_skip)                   // _skip:
    p.GCALL (F_error_skip).             // GCALL error_skip:
      A0    (TR).                       //     n        <= TR
      R0    (ET).                       //     ret.itab => ET
      R1    (EP)                        //     ret.data => EP
    p.JAL   (LB_error, atm.Pn)          // GOTO _error
    p.Label (LB_missing)                // _missing:
    p.GCALL (F_error_missing).          // GCALL error_missing:
      A0    (ET).                       //     t        <= ET
      A1    (UR).                       //     i        <= UR
      A2    (TR).                       //     m        <= TR
      R0    (ET).                       //     ret.itab => ET
      R1    (EP)                        //     ret.data => EP
    p.JAL   (LB_error, atm.Pn)          // GOTO _error
    p.Label (LB_overflow)               // _overflow:
    p.IP    (&_E_overflow, TP)          // TP <= &_E_overflow
    p.LP    (TP, ET)                    // ET <= *TP
    p.ADDPI (TP, 8, TP)                 // TP <=  TP + 8
    p.LP    (TP, EP)                    // EP <= *TP
    p.JAL   (LB_error, atm.Pn)          // GOTO _error
}

func program(p *atm.Builder, s Program) {
    for i, v := range s {
        p.Mark(i)
        translators[v.Op](p, v)
    }
}

func prologue(p *atm.Builder) {
    p.LDAP  (ARG_buf, IP)               // IP <= ARG.buf
    p.LDAQ  (ARG_i, IC)                 // IC <= ARG.i
    p.LDAP  (ARG_p, WP)                 // WP <= ARG.p
    p.LDAP  (ARG_rs, RS)                // RS <= ARG.rs
    p.LDAQ  (ARG_st, ST)                // ST <= ARG.st
}

func epilogue(p *atm.Builder) {
    p.Label (LB_halt)                   // _halt:
    p.MOVP  (atm.Pn, ET)                // ET <= nil
    p.MOVP  (atm.Pn, EP)                // EP <= nil
    p.Label (LB_error)                  // _error:
    p.STRQ  (IC, RET_pos)               // IC => RET.pos
    p.STRP  (ET, RET_err_itab)          // ET => RET.err.itab
    p.STRP  (EP, RET_err_data)          // EP => RET.err.data
    p.HALT  ()                          // HALT
}

var translators = [256]func(*atm.Builder, Instr) {
    OP_int               : translate_OP_int,
    OP_str               : translate_OP_str,
    OP_bin               : translate_OP_bin,
    OP_size              : translate_OP_size,
    OP_type              : translate_OP_type,
    OP_seek              : translate_OP_seek,
    OP_deref             : translate_OP_deref,
    OP_ctr_load          : translate_OP_ctr_load,
    OP_ctr_decr          : translate_OP_ctr_decr,
    OP_ctr_is_zero       : translate_OP_ctr_is_zero,
    OP_map_alloc         : translate_OP_map_alloc,
    OP_map_close         : translate_OP_map_close,
    OP_map_set_i8        : translate_OP_map_set_i8,
    OP_map_set_i16       : translate_OP_map_set_i16,
    OP_map_set_i32       : translate_OP_map_set_i32,
    OP_map_set_i64       : translate_OP_map_set_i64,
    OP_map_set_str       : translate_OP_map_set_str,
    OP_map_set_pointer   : translate_OP_map_set_pointer,
    OP_list_alloc        : translate_OP_list_alloc,
    OP_struct_skip       : translate_OP_struct_skip,
    OP_struct_ignore     : translate_OP_struct_ignore,
    OP_struct_bitmap     : translate_OP_struct_bitmap,
    OP_struct_switch     : translate_OP_struct_switch,
    OP_struct_require    : translate_OP_struct_require,
    OP_struct_is_stop    : translate_OP_struct_is_stop,
    OP_struct_mark_tag   : translate_OP_struct_mark_tag,
    OP_struct_read_type  : translate_OP_struct_read_type,
    OP_struct_check_type : translate_OP_struct_check_type,
    OP_make_state        : translate_OP_make_state,
    OP_drop_state        : translate_OP_drop_state,
    OP_construct         : translate_OP_construct,
    OP_defer             : translate_OP_defer,
    OP_goto              : translate_OP_goto,
    OP_halt              : translate_OP_halt,
}

func translate_OP_int(p *atm.Builder, v Instr) {
    switch v.Iv {
        case 1  : p.ADDP(IP, IC, EP); p.LB(EP, TR);                  p.SB(TR, WP); p.ADDI(IC, 1, IC)    // *WP <= IP[IC++]
        case 2  : p.ADDP(IP, IC, EP); p.LW(EP, TR); p.SWAPW(TR, TR); p.SW(TR, WP); p.ADDI(IC, 2, IC)    // *WP <= bswap16(IP[IC]); IC += 2
        case 4  : p.ADDP(IP, IC, EP); p.LL(EP, TR); p.SWAPL(TR, TR); p.SL(TR, WP); p.ADDI(IC, 4, IC)    // *WP <= bswap32(IP[IC]); IC += 4
        case 8  : p.ADDP(IP, IC, EP); p.LQ(EP, TR); p.SWAPQ(TR, TR); p.SQ(TR, WP); p.ADDI(IC, 8, IC)    // *WP <= bswap64(IP[IC]); IC += 8
        default : panic("can only convert 1, 2, 4 or 8 bytes at a time")
    }
}

func translate_OP_str(p *atm.Builder, _ Instr) {
    p.SP    (atm.Pn, WP)                // *WP <= nil
    translate_OP_binstr(p)              // (read buffer)
}

func translate_OP_bin(p *atm.Builder, _ Instr) {
    p.IP    (&_V_zerovalue, TP)         //  TP <= &_V_zerovalue
    p.SP    (TP, WP)                    // *WP <= TP
    translate_OP_binstr(p)              // (read buffer)
    p.ADDPI (TP, 8, TP)                 //  TP <= TP + 8
    p.SQ    (TR, TP)                    // *TP <= TR
}

func translate_OP_binstr(p *atm.Builder) {
    p.ADDP  (IP, IC, EP)                //  EP <=  IP + IC
    p.ADDI  (IC, 4, IC)                 //  IC <=  IC + 4
    p.LL    (EP, TR)                    //  TR <= *EP
    p.SWAPL (TR, TR)                    //  TR <=  bswap32(TR)
    p.LDAQ  (ARG_nb, UR)                //  UR <= ARG.nb
    p.BLTU  (UR, TR, LB_eof)            //  if UR < TR then GOTO _eof
    p.BEQ   (TR, atm.Rz, "_empty_{n}")  //  if TR == 0 then GOTO _empty_{n}
    p.ADDPI (EP, 4, EP)                 //  EP <=  EP + 4
    p.ADD   (IC, TR, IC)                //  IC <=  IC + TR
    p.SP    (EP, WP)                    // *WP <=  EP
    p.Label ("_empty_{n}")              // _empty_{n}:
    p.ADDPI (WP, 8, TP)                 //  TP <=  WP + 8
    p.SQ    (TR, TP)                    // *TP <=  TR
}

func translate_OP_size(p *atm.Builder, v Instr) {
    p.IQ    (v.Iv, TR)                  // TR <= v.Iv
    p.LDAQ  (ARG_nb, UR)                // UR <= ARG.nb
    p.BLTU  (UR, TR, LB_eof)            // if UR < TR then GOTO _eof
}

func translate_OP_type(p *atm.Builder, v Instr) {
    p.ADDP  (IP, IC, TP)                // TP <=  IP + IC
    p.LB    (TP, TR)                    // TR <= *TP
    p.IB    (int8(v.Tx), UR)            // UR <=  v.Tx
    p.BNE   (TR, UR, LB_type)           // if TR != UR then GOTO _type
    p.ADDI  (IC, 1, IC)                 // IC <=  IC + 1
}

func translate_OP_seek(p *atm.Builder, v Instr) {
    p.ADDPI (WP, v.Iv, WP)              // WP <= WP + v.Iv
}

func translate_OP_deref(p *atm.Builder, v Instr) {
    p.LQ    (WP, TR)                    //  TR <= *WP
    p.BNE   (TR, atm.Rz, "_skip_{n}")   //  if TR != 0 then GOTO _skip_{n}
    p.IB    (1, UR)                     //  UR <= 1
    p.IP    (v.Vt, TP)                  //  TP <= v.Vt
    p.IQ    (int64(v.Vt.Size), TR)      //  TR <= v.Vt.Size
    p.GCALL (F_mallocgc).               //  GCALL mallocgc:
      A0    (TR).                       //      size     <= TR
      A1    (TP).                       //      typ      <= TP
      A2    (UR).                       //      needzero <= UR
      R0    (TP)                        //      ret      => TP
    p.SP    (TP, WP)                    // *WP <= TP
    p.Label ("_skip_{n}")               // _skip_{n}:
    p.LP    (WP, WP)                    //  WP <= *WP
}

func translate_OP_ctr_load(p *atm.Builder, _ Instr) {
    p.ADDP  (IP, IC, EP)                //  EP <=  IP + IC
    p.ADDI  (IC, 4, IC)                 //  IC <=  IC + 4
    p.LL    (EP, TR)                    //  TR <= *EP
    p.SWAPL (TR, TR)                    //  TR <=  bswap32(TR)
    p.ADDP  (RS, ST, TP)                //  TP <=  RS + ST
    p.ADDPI (TP, NbOffset, TP)          //  TP <=  TP + NbOffset
    p.SQ    (TR, TP)                    // *TP <=  TR
}

func translate_OP_ctr_decr(p *atm.Builder, _ Instr) {
    p.ADDP  (RS, ST, TP)                //  TP <=  RS + ST
    p.ADDPI (TP, NbOffset, TP)          //  TP <=  TP + NbOffset
    p.LQ    (TP, TR)                    //  TR <= *TP
    p.SUBI  (TR, 1, TR)                 //  TR <=  TR - 1
    p.SQ    (TR, TP)                    // *TP <=  TR
}

func translate_OP_ctr_is_zero(p *atm.Builder, v Instr) {
    p.ADDP  (RS, ST, TP)                // TP <=  RS + ST
    p.ADDPI (TP, NbOffset, TP)          // TP <=  TP + NbOffset
    p.LQ    (TP, TR)                    // TR <= *TP
    p.BEQ   (TR, atm.Rz, p.At(v.To))    // if TR == 0 then GOTO @v.To
}

func translate_OP_map_alloc(p *atm.Builder, v Instr) {
    p.ADDP  (RS, ST, TP)                //  TP <=  RS + ST
    p.ADDPI (TP, NbOffset, TP)          //  TP <=  TP + NbOffset
    p.LQ    (TP, TR)                    //  TR <= *TP
    p.LP    (WP, TP)                    //  TP <= *WP
    p.IP    (v.Vt, ET)                  //  ET <=  v.Vt
    p.GCALL (F_makemap).                //  GCALL makemap:
      A0    (ET).                       //      t    <= ET
      A1    (TR).                       //      hint <= TR
      A2    (TP).                       //      h    <= TP
      R0    (TP)                        //      ret  => TP
    p.SP    (TP, WP)                    // *WP <=  TP
    p.ADDP  (RS, ST, EP)                //  EP <=  RS + ST
    p.ADDPI (EP, MpOffset, EP)          //  EP <=  EP + MpOffset
    p.SP    (TP, EP)                    // *EP <=  TP
}

func translate_OP_map_close(p *atm.Builder, _ Instr) {
    p.ADDP  (RS, ST, TP)                //  TP <= RS + ST
    p.ADDPI (TP, MpOffset, TP)          //  TP <= TP + MpOffset
    p.SP    (atm.Pn, TP)                // *TP <= nil
}

func translate_OP_map_set_i8(p *atm.Builder, v Instr) {
    p.ADDP  (IP, IC, EP)                // EP <=  IP + IC
    p.ADDP  (RS, ST, TP)                // TP <=  RS + ST
    p.ADDPI (TP, MpOffset, TP)          // TP <=  TP + MpOffset
    p.LP    (TP, TP)                    // TP <= *TP
    p.IP    (v.Vt, ET)                  // ET <=  v.Vt
    p.GCALL (F_mapassign).              // GCALL mapassign:
      A0    (ET).                       //     t   <= ET
      A1    (TP).                       //     h   <= TP
      A2    (EP).                       //     key <= EP
      R0    (WP)                        //     ret => WP
    p.ADDI  (IC, 1, IC)                 // IC <=  IC + 1
}

func translate_OP_map_set_i16(p *atm.Builder, v Instr) {
    p.ADDP  (IP, IC, ET)                //  ET <=  IP + IC
    p.ADDI  (IC, 2, IC)                 //  IC <=  IC + 2
    p.ADDP  (RS, ST, TP)                //  TP <=  RS + ST
    p.ADDPI (TP, MpOffset, TP)          //  TP <=  TP + MpOffset
    p.LP    (TP, EP)                    //  EP <= *TP
    p.ADDPI (RS, IvOffset, TP)          //  TP <=  RS + IvOffset
    p.LW    (ET, TR)                    //  ET <= *LP
    p.SWAPW (TR, TR)                    //  TR <=  bswap16(TR)
    p.SW    (TR, TP)                    // *TP <=  TR
    p.IP    (v.Vt, ET)                  //  ET <=  v.Vt
    p.GCALL (F_mapassign).              // GCALL mapassign:
      A0    (ET).                       //     t   <= ET
      A1    (EP).                       //     h   <= EP
      A2    (TP).                       //     key <= TP
      R0    (WP)                        //     ret => WP
}

func translate_OP_map_set_i32(p *atm.Builder, v Instr) {
    if rt.MapType(v.Vt).IsFastMap() {
        translate_OP_map_set_i32_fast(p, v)
    } else {
        translate_OP_map_set_i32_safe(p, v)
    }
}

func translate_OP_map_set_i32_fast(p *atm.Builder, v Instr) {
    p.ADDP  (IP, IC, EP)                // EP <=  IP + IC
    p.ADDI  (IC, 4, IC)                 // IC <=  IC + 4
    p.ADDP  (RS, ST, TP)                // TP <=  RS + ST
    p.ADDPI (TP, MpOffset, TP)          // TP <=  TP + MpOffset
    p.LP    (TP, TP)                    // TP <= *TP
    p.LL    (EP, TR)                    // TR <= *EP
    p.SWAPL (TR, TR)                    // TR <=  bswap32(TR)
    p.IP    (v.Vt, ET)                  // ET <=  v.Vt
    p.GCALL (F_mapassign_fast32).       // GCALL mapassign_fast32:
      A0    (ET).                       //     t   <= ET
      A1    (TP).                       //     h   <= TP
      A2    (TR).                       //     key <= TR
      R0    (WP)                        //     ret => WP
}

func translate_OP_map_set_i32_safe(p *atm.Builder, v Instr) {
    p.ADDP  (IP, IC, ET)                //  ET <=  IP + IC
    p.ADDI  (IC, 4, IC)                 //  IC <=  IC + 4
    p.ADDP  (RS, ST, TP)                //  TP <=  RS + ST
    p.ADDPI (TP, MpOffset, TP)          //  TP <=  TP + MpOffset
    p.LP    (TP, EP)                    //  EP <= *TP
    p.ADDPI (RS, IvOffset, TP)          //  TP <=  RS + IvOffset
    p.LL    (ET, TR)                    //  TR <= *ET
    p.SWAPL (TR, TR)                    //  TR <=  bswap32(TR)
    p.SL    (TR, TP)                    // *TP <=  TR
    p.IP    (v.Vt, ET)                  //  ET <=  v.Vt
    p.GCALL (F_mapassign).              // GCALL mapassign:
      A0    (ET).                       //     t   <= ET
      A1    (EP).                       //     h   <= EP
      A2    (TP).                       //     key <= TP
      R0    (WP)                        //     ret => WP
}

func translate_OP_map_set_i64(p *atm.Builder, v Instr) {
    if rt.MapType(v.Vt).IsFastMap() {
        translate_OP_map_set_i64_fast(p, v)
    } else {
        translate_OP_map_set_i64_safe(p, v)
    }
}

func translate_OP_map_set_i64_fast(p *atm.Builder, v Instr) {
    p.ADDP  (IP, IC, EP)                // EP <=  IP + IC
    p.ADDI  (IC, 8, IC)                 // IC <=  IC + 8
    p.ADDP  (RS, ST, TP)                //  TP <=  RS + ST
    p.ADDPI (TP, MpOffset, TP)          //  TP <=  TP + MpOffset
    p.LP    (TP, TP)                    // TP <= *TP
    p.LQ    (EP, TR)                    // TR <= *EP
    p.SWAPQ (TR, TR)                    // TR <=  bswap64(TR)
    p.IP    (v.Vt, ET)                  // ET <=  v.Vt
    p.GCALL (F_mapassign_fast64).       // GCALL mapassign_fast64:
      A0    (ET).                       //     t   <= ET
      A1    (TP).                       //     h   <= TP
      A2    (TR).                       //     key <= TR
      R0    (WP)                        //     ret => WP
}

func translate_OP_map_set_i64_safe(p *atm.Builder, v Instr) {
    p.ADDP  (IP, IC, ET)                //  ET <=  IP + IC
    p.ADDI  (IC, 8, IC)                 //  IC <=  IC + 8
    p.ADDP  (RS, ST, TP)                //  TP <=  RS + ST
    p.ADDPI (TP, MpOffset, TP)          //  TP <=  TP + MpOffset
    p.LP    (TP, EP)                    //  EP <= *TP
    p.ADDPI (RS, IvOffset, TP)          //  TP <=  RS + IvOffset
    p.LQ    (ET, TR)                    //  TR <= *ET
    p.SWAPQ (TR, TR)                    //  TR <=  bswap64(TR)
    p.SQ    (TR, TP)                    // *TP <=  TR
    p.IP    (v.Vt, ET)                  //  ET <=  v.Vt
    p.GCALL (F_mapassign).              // GCALL mapassign:
      A0    (ET).                       //     t   <= ET
      A1    (EP).                       //     h   <= EP
      A2    (TP).                       //     key <= TP
      R0    (WP)                        //     ret => WP
}

func translate_OP_map_set_str(p *atm.Builder, v Instr) {
    if rt.MapType(v.Vt).IsFastMap() {
        translate_OP_map_set_str_fast(p, v)
    } else {
        translate_OP_map_set_str_safe(p, v)
    }
}

func translate_OP_map_set_str_fast(p *atm.Builder, v Instr) {
    p.ADDP  (IP, IC, EP)                // EP <=  IP + IC
    p.ADDI  (IC, 4, IC)                 // IC <=  IC + 4
    p.LL    (EP, TR)                    // TR <= *EP
    p.SWAPL (TR, TR)                    // TR <=  bswap32(TR)
    p.LDAQ  (ARG_nb, UR)                // UR <=  ARG.nb
    p.BLTU  (UR, TR, LB_eof)            // if UR < TR then GOTO _eof
    p.MOVP  (atm.Pn, EP)                // EP <=  nil
    p.BEQ   (TR, atm.Rz, "_empty_{n}")  // if TR == 0 then GOTO _empty_{n}
    p.ADDP  (IP, IC, EP)                // EP <=  IP + IC
    p.ADD   (IC, TR, IC)                // IC <=  IC + TR
    p.Label ("_empty_{n}")              // _empty_{n}:
    p.ADDP  (RS, ST, TP)                //  TP <=  RS + ST
    p.ADDPI (TP, MpOffset, TP)          //  TP <=  TP + MpOffset
    p.LP    (TP, TP)                    // TP <= *TP
    p.IP    (v.Vt, ET)                  // ET <=  v.Vt
    p.GCALL (F_mapassign_faststr).      // GCALL mapassign_faststr:
      A0    (ET).                       //     t     <= ET
      A1    (TP).                       //     h     <= TP
      A2    (EP).                       //     s.ptr <= EP
      A3    (TR).                       //     s.len <= TR
      R0    (WP)                        //     ret   => WP
}

func translate_OP_map_set_str_safe(p *atm.Builder, v Instr) {
    p.ADDP  (IP, IC, ET)                //  ET <=  IP + IC
    p.ADDI  (IC, 4, IC)                 //  IC <=  IC + 4
    p.LL    (ET, TR)                    //  TR <= *ET
    p.SWAPL (TR, TR)                    //  TR <=  bswap32(TR)
    p.LDAQ  (ARG_nb, UR)                //  UR <= ARG.nb
    p.BLTU  (UR, TR, LB_eof)            //  if UR < TR then GOTO _eof
    p.ADDPI (RS, IvOffset, TP)          //  TP <=  RS + IvOffset
    p.SQ    (TR, TP)                    // *TP <=  TR
    p.ADDPI (RS, PrOffset, TP)          //  TP <=  RS + PrOffset
    p.SP    (atm.Pn, TP)                // *TP <=  nil
    p.BEQ   (TR, atm.Rz, "_empty_{n}")  //  if TR == 0 then GOTO _empty_{n}
    p.ADDPI (ET, 4, ET)                 //  ET <=  ET + 4
    p.ADD   (IC, TR, IC)                //  IC <=  IC + TR
    p.SP    (ET, TP)                    // *TP <=  ET
    p.Label ("_empty_{n}")              // _empty_{n}:
    p.ADDP  (RS, ST, EP)                //  EP <=  RS + ST
    p.ADDPI (EP, MpOffset, EP)          //  EP <=  EP + MpOffset
    p.LP    (EP, EP)                    //  EP <= *EP
    p.IP    (v.Vt, ET)                  //  ET <=  v.Vt
    p.GCALL (F_mapassign).              //  GCALL mapassign:
      A0    (ET).                       //      t   <= ET
      A1    (EP).                       //      h   <= EP
      A2    (TP).                       //      key <= TP
      R0    (WP)                        //      ret => WP
    p.SP    (atm.Pn, TP)                // *TP <=  nil
}

func translate_OP_map_set_pointer(p *atm.Builder, v Instr) {
    if rt.MapType(v.Vt).IsFastMap() {
        translate_OP_map_set_pointer_fast(p, v)
    } else {
        translate_OP_map_set_pointer_safe(p, v)
    }
}

func translate_OP_map_set_pointer_fast(p *atm.Builder, v Instr) {
    p.ADDP  (RS, ST, TP)                //  TP <=  RS + ST
    p.ADDPI (TP, MpOffset, TP)          //  TP <=  TP + MpOffset
    p.LP    (TP, TP)                    // TP <= *TP
    p.IP    (v.Vt, ET)                  // ET <=  v.Vt
    p.GCALL (F_mapassign_fast64ptr).    // GCALL mapassign_fast64ptr:
      A0    (ET).                       //     t   <= ET
      A1    (TP).                       //     h   <= TP
      A2    (WP).                       //     key <= WP
      R0    (WP)                        //     ret => WP
}

func translate_OP_map_set_pointer_safe(p *atm.Builder, v Instr) {
    p.ADDP  (RS, ST, TP)                //  TP <=  RS + ST
    p.ADDPI (TP, MpOffset, TP)          //  TP <=  TP + MpOffset
    p.LP    (TP, EP)                    //  EP <= *TP
    p.ADDPI (RS, PrOffset, TP)          //  TP <=  RS + PrOffset
    p.SP    (WP, TP)                    // *TP <=  WP
    p.IP    (v.Vt, ET)                  //  ET <=  v.Vt
    p.GCALL (F_mapassign).              //  GCALL mapassign:
      A0    (ET).                       //      t   <= ET
      A1    (EP).                       //      h   <= EP
      A2    (TP).                       //      key <= TP
      R0    (WP)                        //      ret => WP
    p.SP    (atm.Pn, TP)                // *TP <=  nil
}

func translate_OP_list_alloc(p *atm.Builder, v Instr) {
    p.ADDP  (RS, ST, TP)                //  TP <=  RS + ST
    p.ADDPI (TP, NbOffset, TP)          //  TP <=  TP + NbOffset
    p.LQ    (TP, TR)                    //  TR <= *TP
    p.ADDPI (WP, 8, TP)                 //  TP <=  WP + 8
    p.SQ    (TR, TP)                    // *TP <=  TR
    p.ADDPI (TP, 8, TP)                 //  TP <=  TP + 8
    p.LQ    (TP, UR)                    //  UR <= *TP
    p.BGEU  (UR, TR, "_noalloc_{n}")    //  if UR >= TR then GOTO _noalloc_{n}
    p.SQ    (TR, TP)                    // *TP <=  TR
    p.IP    (&_V_zerovalue, TP)         //  TP <=  &_V_zerovalue
    p.BEQ   (TR, atm.Rz, "_empty_{n}")  //  if TR == 0 then GOTO _empty_{n}
    p.IB    (1, UR)                     //  UR <=  1
    p.IP    (v.Vt, TP)                  //  TP <=  v.Vt
    p.MULI  (TR, int64(v.Vt.Size), TR)  //  TR <=  TR * v.Vt.Size
    p.GCALL (F_mallocgc).               //  GCALL mallocgc:
      A0    (TR).                       //      size     <= TR
      A1    (TP).                       //      typ      <= TP
      A2    (UR).                       //      needzero <= UR
      R0    (TP)                        //      ret      => TP
    p.Label ("_empty_{n}")              // _empty_{n}:
    p.SP    (TP, WP)                    // *WP <= TP
    p.Label ("_noalloc_{n}")            // _noalloc_{n}:
    p.LP    (WP, WP)                    //  WP <= *WP
}

func translate_OP_struct_skip(p *atm.Builder, _ Instr) {
    p.ADDPI (RS, SkOffset, TP)          // TP <= RS + SkOffset
    p.LDAQ  (ARG_nb, TR)                // TR <= ARG.nb
    p.SUB   (TR, IC, TR)                // TR <= TR - IC
    p.ADDP  (IP, IC, EP)                // EP <= IP + IC
    p.CCALL (C_skip).                   // CCALL skip:
      A0    (TP).                       //     st  <= TP
      A1    (EP).                       //     s   <= EP
      A2    (TR).                       //     n   <= TR
      A3    (TG).                       //     t   <= TG
      R0    (TR)                        //     ret => TR
    p.BLT   (TR, atm.Rz, LB_skip)       // if TR < 0 then GOTO _skip
    p.ADD   (IC, TR, IC)                // IC <= IC + TR
}

func translate_OP_struct_ignore(p *atm.Builder, _ Instr) {
    p.ADDPI (RS, SkOffset, TP)          // TP <= RS + SkOffset
    p.LDAQ  (ARG_nb, TR)                // TR <= ARG.nb
    p.SUB   (TR, IC, TR)                // TR <= TR - IC
    p.ADDP  (IP, IC, EP)                // EP <= IP + IC
    p.IB    (int8(defs.T_struct), TG)   // TG <= defs.T_struct
    p.CCALL (C_skip).                   // CCALL skip:
      A0    (TP).                       //     st  <= TP
      A1    (EP).                       //     s   <= EP
      A2    (TR).                       //     n   <= TR
      A3    (TG).                       //     t   <= TG
      R0    (TR)                        //     ret => TR
    p.BLT   (TR, atm.Rz, LB_skip)       // if TR < 0 then GOTO _skip
    p.ADD   (IC, TR, IC)                // IC <= IC + TR
}

func translate_OP_struct_bitmap(p *atm.Builder, v Instr) {
    buf := newFieldBitmap()
    tab := mkstab(v.Sw, v.Iv)

    /* add all the bits */
    for _, i := range tab {
        buf.Append(i)
    }

    /* clear bits of required fields if any */
    for i := int64(0); i < MaxBitmap; i++ {
        if buf[i] != 0 {
            p.ADDP  (RS, ST, TP)                //  TP <= RS + ST
            p.ADDPI (TP, FmOffset + i * 8, TP)  //  TP <= TP + FmOffset + i * 8
            p.SQ    (atm.Rz, TP)                // *TP <= 0
        }
    }

    /* release the buffer */
    buf.Clear()
    buf.Free()
}

func translate_OP_struct_switch(p *atm.Builder, v Instr) {
    stab := mkstab(v.Sw, v.Iv)
    ptab := make([]string, v.Iv)

    /* convert the switch table */
    for i, to := range stab {
        if to >= 0 {
            ptab[i] = p.At(to)
        }
    }

    /* load and dispatch the field */
    p.ADDP  (IP, IC, EP)                // EP <=  IP + IC
    p.ADDI  (IC, 2, IC)                 // IC <=  IC + 2
    p.LW    (EP, TR)                    // TR <= *EP
    p.SWAPW (TR, TR)                    // TR <=  bswap16(TR)
    p.BSW   (TR, ptab)                  // switch TR on ptab
}

func translate_OP_struct_require(p *atm.Builder, v Instr) {
    buf := newFieldBitmap()
    tab := mkstab(v.Sw, v.Iv)

    /* add all the bits */
    for _, i := range tab {
        buf.Append(i)
    }

    /* test mask for each word if any */
    for i := int64(0); i < MaxBitmap; i++ {
        if buf[i] != 0 {
            p.ADDP  (RS, ST, TP)                // TP <=  RS + ST
            p.ADDPI (TP, FmOffset + i * 8, TP)  // TP <=  TP + FmOffset + i * 8
            p.LQ    (TP, TR)                    // TR <= *TP
            p.ANDI  (TR, buf[i], TR)            // TR <=  TR & buf[i]
            p.XORI  (TR, buf[i], TR)            // TR <=  TR ^ buf[i]
            p.IQ    (i, UR)                     // UR <=  i
            p.IP    (v.Vt, ET)                  // ET <=  v.Vt
            p.BNE   (TR, atm.Rz, LB_missing)    // if TR != 0 then GOTO _missing
        }
    }

    /* release the buffer */
    buf.Clear()
    buf.Free()
}

func translate_OP_struct_is_stop(p *atm.Builder, v Instr) {
    p.BEQ   (TG, atm.Rz, p.At(v.To))    // if TG == 0 then GOTO @v.To
}

func translate_OP_struct_mark_tag(p *atm.Builder, v Instr) {
    p.ADDP  (RS, ST, TP)                //  TP <=  RS + ST
    p.ADDPI (TP, FmOffset, TP)          //  TP <=  TP + FmOffset
    p.ADDPI (TP, v.Iv / 64 * 8, TP)     //  TP <=  TP + v.Iv / 64 * 8
    p.LQ    (TP, TR)                    //  TR <= *TP
    p.SBITI (TR, v.Iv % 64, TR)         //  TR <=  TR | (1 << (v.Iv % 64))
    p.SQ    (TR, TP)                    // *TP <=  TR
}

func translate_OP_struct_read_type(p *atm.Builder, _ Instr) {
    p.ADDP  (IP, IC, EP)                //  EP <=  IP + IC
    p.ADDI  (IC, 1, IC)                 //  IC <=  IC + 1
    p.LB    (EP, TG)                    //  TG <= *EP
}

func translate_OP_struct_check_type(p *atm.Builder, v Instr) {
    p.IB    (int8(v.Tx), TR)            // UR <= v.Tx
    p.BNE   (TG, TR, p.At(v.To))        // if TG != TR then GOTO @v.To
}

func translate_OP_make_state(p *atm.Builder, _ Instr) {
    p.IQ    (StateMax, TR)              //  TR <= StateMax
    p.BGEU  (ST, TR, LB_overflow)       //  if ST >= TR then GOTO _overflow
    p.ADDP  (RS, ST, TP)                //  TP <= RS + ST
    p.ADDPI (TP, WpOffset, TP)          //  TP <= TP + WpOffset
    p.SP    (WP, TP)                    // *TP <= WP
    p.ADDI  (ST, StateSize, ST)         //  ST <= ST + StateSize
}

func translate_OP_drop_state(p *atm.Builder, _ Instr) {
    p.SUBI  (ST, StateSize, ST)         //  ST <=  ST - StateSize
    p.ADDP  (RS, ST, TP)                //  TP <=  RS + ST
    p.ADDPI (TP, WpOffset, TP)          //  TP <=  TP + WpOffset
    p.LP    (TP, WP)                    //  WP <= *TP
    p.SP    (atm.Pn, TP)                // *TP <=  nil
}

func translate_OP_construct(p *atm.Builder, v Instr) {
    p.IB    (1, UR)                     // UR <= 1
    p.IP    (v.Vt, TP)                  // TP <= v.Vt
    p.IQ    (int64(v.Vt.Size), TR)      // TR <= v.Vt.Size
    p.GCALL (F_mallocgc).               // GCALL mallocgc:
      A0    (TR).                       //     size     <= TR
      A1    (TP).                       //     typ      <= TP
      A2    (UR).                       //     needzero <= UR
      R0    (WP)                        //     ret      => WP
}

func translate_OP_defer(p *atm.Builder, v Instr) {
    p.IP    (v.Vt, TP)                  // TP <= v.Vt
    p.LDAQ  (ARG_nb, TR)                // TR <= ARG.nb
    p.GCALL (F_decode).                 // GCALL decode:
      A0    (TP).                       //     vt       <= TP
      A1    (IP).                       //     buf      <= IP
      A2    (TR).                       //     nb       <= TR
      A3    (IC).                       //     i        <= IC
      A4    (WP).                       //     p        <= WP
      A5    (RS).                       //     rs       <= RS
      A6    (ST).                       //     st       <= ST
      R0    (IC).                       //     pos      => IC
      R1    (ET).                       //     err.type => ET
      R2    (EP)                        //     err.data => EP
    p.BNEN  (ET, LB_error)              // if ET != nil then GOTO _error
}

func translate_OP_goto(p *atm.Builder, v Instr) {
    p.JAL   (p.At(v.To), atm.Pn)        // GOTO @v.To
}

func translate_OP_halt(p *atm.Builder, _ Instr) {
    p.JAL   (LB_halt, atm.Pn)           // GOTO _halt
}
