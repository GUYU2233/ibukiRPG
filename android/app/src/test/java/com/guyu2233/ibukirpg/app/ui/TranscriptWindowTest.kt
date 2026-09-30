package com.guyu2233.ibukirpg.app.ui

import com.guyu2233.ibukirpg.app.data.EntryV1
import com.guyu2233.ibukirpg.app.ui.game.TranscriptWindow
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class TranscriptWindowTest {
    private fun e(id: Long) = EntryV1(id = id, kind = "narration", text = "t$id")

    @Test fun appendDedupesAndCaps() {
        val cur = (1L..5).map(::e)
        val (out, trimmed) = TranscriptWindow.append(cur, listOf(e(5), e(6), e(7)), max = 6)
        assertEquals((2L..7).toList(), out.map { it.id })
        assertTrue(trimmed)
        val (out2, trimmed2) = TranscriptWindow.append(cur, listOf(e(6)), max = 10)
        assertEquals(6, out2.size)
        assertFalse(trimmed2)
    }

    @Test fun prependKeepsOrder() {
        val cur = (11L..13).map(::e)
        val out = TranscriptWindow.prepend(cur, listOf(e(10), e(8), e(9), e(11)))
        assertEquals(listOf(8L, 9, 10, 11, 12, 13), out.map { it.id })
        assertEquals(8L, TranscriptWindow.cursor(out))
    }
}
