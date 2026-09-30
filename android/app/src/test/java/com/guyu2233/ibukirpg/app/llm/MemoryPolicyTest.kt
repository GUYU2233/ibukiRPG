package com.guyu2233.ibukirpg.app.llm

import com.guyu2233.ibukirpg.app.llm.MemoryPolicy.MB
import org.junit.Assert.assertTrue
import org.junit.Test

class MemoryPolicyTest {
    private val gb = 1024 * MB

    @Test fun smallModelOnBigPhoneIsOk() {
        val d = MemoryPolicy.decide(1100 * MB, 2048, availMem = 4 * gb, totalMem = 8 * gb, lowMemory = false)
        assertTrue(d is MemoryPolicy.Decision.Ok)
    }

    @Test fun warnsWhenAvailableMemoryIsShort() {
        val d = MemoryPolicy.decide(2 * gb, 4096, availMem = 1 * gb, totalMem = 8 * gb, lowMemory = false)
        assertTrue(d is MemoryPolicy.Decision.Warn)
        assertTrue(MemoryPolicy.decide(500 * MB, 1024, 4 * gb, 8 * gb, lowMemory = true) is MemoryPolicy.Decision.Warn)
    }

    @Test fun refusesModelTooLargeForDevice() {
        val d = MemoryPolicy.decide(2 * gb, 4096, availMem = 3 * gb, totalMem = 4 * gb, lowMemory = false)
        assertTrue(d is MemoryPolicy.Decision.Refuse)
        assertTrue((d as MemoryPolicy.Decision.Refuse).message.contains("GB"))
    }

    @Test fun contextIncreasesEstimate() {
        assertTrue(MemoryPolicy.estimate(gb, 8192) > MemoryPolicy.estimate(gb, 1024))
    }
}
