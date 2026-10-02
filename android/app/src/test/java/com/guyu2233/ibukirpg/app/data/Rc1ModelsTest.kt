package com.guyu2233.ibukirpg.app.data

import kotlinx.serialization.ExperimentalSerializationApi
import kotlinx.serialization.encodeToString
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonNamingStrategy
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

/** 0.2.0-rc1 新增字段与 Go 端 JSON（snake_case）一致。 */
@OptIn(ExperimentalSerializationApi::class)
class Rc1ModelsTest {
    private val json = Json {
        ignoreUnknownKeys = true
        explicitNulls = false
        coerceInputValues = true
        encodeDefaults = true
        namingStrategy = JsonNamingStrategy.SnakeCase
    }

    private fun res(name: String) = javaClass.classLoader!!.getResource("fixtures/$name")!!.readText()

    @Test fun genSettingsWorldSimKeys() {
        val s = json.encodeToString(GenSettingsV1(simEveryMinutes = -1, simAiCallsPerDay = 2, worldToolsOff = true))
        assertTrue(s, s.contains("\"sim_every_minutes\":-1"))
        assertTrue(s, s.contains("\"sim_ai_calls_per_day\":2"))
        assertTrue(s, s.contains("\"world_tools_off\":true"))
        assertEquals(0, json.decodeFromString<GenSettingsV1>("{}").simEveryMinutes)
    }

    @Test fun revertPlanFixture() {
        val p = json.decodeFromString<RevertPlanV1>(res("v02_revert_plan.json"))
        assertTrue(p.dependents.isNotEmpty())
        assertTrue(p.canSingle)
        assertTrue(!p.singleNote.isNullOrBlank())
    }

    @Test fun simEntryAndExternalNotice() {
        val b = json.decodeFromString<GameBundle>(res("v02_sim.json"))
        val sim = b.transcript.last { it.kind == "sim" }
        assertTrue(sim.text.isNotBlank())
        assertTrue(sim.world?.changes?.isNotEmpty() == true)
        val withExt = json.decodeFromString<GameBundle>("""{"external_notice":{"count":2,"text":"外部工具修改了 2 项设定","source":"mcp"}}""")
        assertEquals(2, withExt.externalNotice?.count)
        assertEquals("mcp", withExt.externalNotice?.source)
        val d = json.decodeFromString<DecisionV1>("""{"id":"dec-sim-1","world":true}""")
        assertTrue(d.world)
    }
}
