package com.guyu2233.ibukirpg.app

import android.app.Application
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onRoot
import com.github.takahirom.roborazzi.captureRoboImage
import com.guyu2233.ibukirpg.app.data.AIStatusV1
import com.guyu2233.ibukirpg.app.data.AppSettings
import com.guyu2233.ibukirpg.app.data.CharacterV1
import com.guyu2233.ibukirpg.app.data.GameBundle
import com.guyu2233.ibukirpg.app.data.InventoryV1
import com.guyu2233.ibukirpg.app.data.JournalEntryV1
import com.guyu2233.ibukirpg.app.data.NPCV1
import com.guyu2233.ibukirpg.app.data.PackV1
import com.guyu2233.ibukirpg.app.data.SlotV1
import com.guyu2233.ibukirpg.app.data.ThemeMode
import com.guyu2233.ibukirpg.app.ui.game.GameContent
import com.guyu2233.ibukirpg.app.ui.game.GameState
import com.guyu2233.ibukirpg.app.ui.game.PanelsContent
import com.guyu2233.ibukirpg.app.ui.game.PanelsState
import com.guyu2233.ibukirpg.app.ui.home.HomeContent
import com.guyu2233.ibukirpg.app.ui.home.HomeState
import com.guyu2233.ibukirpg.app.ui.packs.PacksContent
import com.guyu2233.ibukirpg.app.ui.packs.PacksState
import com.guyu2233.ibukirpg.app.ui.saves.SavesContent
import com.guyu2233.ibukirpg.app.ui.saves.SavesState
import com.guyu2233.ibukirpg.app.ui.settings.AIForm
import com.guyu2233.ibukirpg.app.ui.settings.SettingsActions
import com.guyu2233.ibukirpg.app.ui.settings.SettingsContent
import com.guyu2233.ibukirpg.app.ui.settings.TestState
import com.guyu2233.ibukirpg.app.ui.theme.IbukiTheme
import kotlinx.serialization.ExperimentalSerializationApi
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonNamingStrategy
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config
import org.robolectric.annotation.GraphicsMode
import java.io.File

/**
 * JVM 截图测试：用 Robolectric（原生图形模式）渲染真实的界面 Composable。
 * 游戏数据来自 `go run ./cmd/uifixtures` —— 真实 Go 引擎离线试玩 5 回合后导出的 JSON（含故事包列表与 HUD）。
 * 输出目录：<repo>/build/screenshots
 */
@RunWith(RobolectricTestRunner::class)
@GraphicsMode(GraphicsMode.Mode.NATIVE)
@Config(sdk = [35], application = Application::class, qualifiers = "w411dp-h891dp-xxhdpi")
class ScreenshotTest {
    @get:Rule val compose = createComposeRule()

    @OptIn(ExperimentalSerializationApi::class)
    private val json = Json {
        ignoreUnknownKeys = true
        explicitNulls = false
        coerceInputValues = true
        namingStrategy = JsonNamingStrategy.SnakeCase
    }

    private inline fun <reified T> fixture(name: String): T {
        val text = javaClass.classLoader!!.getResource("fixtures/$name")!!.readText()
        return json.decodeFromString(text)
    }

    private fun out(name: String): String {
        val dir = File(System.getProperty("ibuki.shots.dir") ?: "build/screenshots").apply { mkdirs() }
        return File(dir, name).absolutePath
    }

    private fun shoot(name: String, dark: Boolean = false, content: @Composable () -> Unit) {
        compose.setContent {
            IbukiTheme(theme = if (dark) ThemeMode.DARK else ThemeMode.LIGHT, dynamicColor = false) {
                Surface(Modifier.fillMaxSize(), color = MaterialTheme.colorScheme.background) { content() }
            }
        }
        compose.waitForIdle()
        compose.onRoot().captureRoboImage(out(name))
    }

    private val slots: List<SlotV1> get() = fixture("saves.json")

    private val game: GameState get() {
        val b: GameBundle = fixture("game.json")
        return GameState(loading = false, scene = b.scene, entries = b.transcript, suggestions = b.suggestions)
    }

    private val panels: PanelsState get() = PanelsState(
        character = fixture<CharacterV1>("character.json"),
        inventory = fixture<InventoryV1>("inventory.json"),
        npcs = fixture<List<NPCV1>>("npcs.json"),
        journal = fixture<List<JournalEntryV1>>("journal.json"),
    )

    /** 内置故事包 + 一个模拟的“已导入”故事包 + 一个不兼容的导入包（展示错误状态）。 */
    private val packs: PacksState get() {
        val builtin: List<PackV1> = fixture("packs.json")
        val imported = PackV1(
            id = "moon_well", name = "月井奇谭", version = "1.0.0", type = "story", author = "玩家投稿",
            tagline = "井底的月亮会说话。", description = "一口古井，一个只在满月出现的倒影。社区制作的示例故事包。",
            tags = listOf("悬疑", "社区"), icon = "star", accent = "#4A5AA8", builtin = false, playable = true,
        )
        val broken = PackV1(
            id = "future_pack", name = "星海远航", version = "2.0.0", type = "story", author = "某作者",
            tagline = "需要更新的引擎版本。", builtin = false, playable = false,
            error = "需要引擎版本 >=0.3.0，当前是 0.1.2rc1。请先更新 App。", accent = "#37474F",
        )
        return PacksState(loading = false, packs = builtin + imported + broken)
    }

    private val home: HomeState get() {
        val s = slots
        return HomeState(loading = false, latest = s.firstOrNull(), saveCount = s.size, ai = AIStatusV1(kind = "offline"), version = "0.1.2rc1")
    }

    @Test fun home() = shoot("01-home.png") {
        HomeContent(home, {}, {}, {}, {}, {})
    }

    @Test fun homeDark() = shoot("02-home-dark.png", dark = true) {
        HomeContent(home, {}, {}, {}, {}, {})
    }

    @Test fun gameScreen() = shoot("03-game.png") {
        GameContent(game, "", PanelsState(), {}, {}, {}, { _, _ -> }, {}, {}, {})
    }

    @Test fun gameDarkTyping() = shoot("04-game-dark.png", dark = true) {
        GameContent(game, "我想问问米拉刚才看见了什么", PanelsState(), {}, {}, {}, { _, _ -> }, {}, {}, {})
    }

    @Test fun panelsPeople() = shoot("05-panel-people.png") {
        PanelsContent(panels, busy = false, onAction = { _, _ -> }, initialTab = 2)
    }

    @Test fun panelsInventory() = shoot("06-panel-inventory.png") {
        PanelsContent(panels, busy = false, onAction = { _, _ -> }, initialTab = 1)
    }

    @Test fun panelsCharacter() = shoot("07-panel-character.png") {
        PanelsContent(panels, busy = false, onAction = { _, _ -> }, initialTab = 0)
    }

    @Test fun packPicker() = shoot("10-packs.png") {
        PacksContent(packs, {}, {}, { _, _ -> }, {}, {}, {})
    }

    @Test fun packPickerDark() = shoot("11-packs-dark.png", dark = true) {
        PacksContent(packs.copy(packs = packs.packs.drop(1)), {}, {}, { _, _ -> }, {}, {}, {})
    }

    @Test fun gameHudExpanded() = shoot("12-game-hud-expanded.png") {
        GameContent(game, "", PanelsState(), {}, {}, {}, { _, _ -> }, {}, {}, {}, initialHudExpanded = true)
    }

    @Test fun gameHudDarkExpanded() = shoot("13-game-hud-dark.png", dark = true) {
        GameContent(game, "", PanelsState(), {}, {}, {}, { _, _ -> }, {}, {}, {}, initialHudExpanded = true)
    }

    @Test fun saves() = shoot("14-saves.png") {
        val list = slots + slots.last().copy(
            id = "missing", name = "月井奇谭 · 旧存档", packId = "moon_well", packName = "月井奇谭", packVersion = "1.0.0",
            location = "古井边", story = null, current = false, packProblem = "这个存档使用的故事包 \"moon_well\"（1.0.0）没有安装或已被删除。重新导入该故事包后即可继续。",
        )
        SavesContent(SavesState(loading = false, slots = list), {}, {}, { _, _ -> }, { _, _ -> }, { _, _ -> }, {})
    }

    @Test fun settings() = shoot("08-settings.png") {
        SettingsContent(
            settings = AppSettings(aiKind = "deepseek"),
            form = AIForm(kind = "deepseek", baseUrl = "https://api.deepseek.com", model = "deepseek-chat", hasSavedKey = true, test = TestState.Idle),
            saved = false,
            engineVersion = "0.1.2rc1",
            actions = SettingsActions(),
        )
    }

    @Test fun settingsOffline() = shoot("09-settings-offline.png") {
        SettingsContent(
            settings = AppSettings(),
            form = AIForm(kind = "offline"),
            saved = false,
            engineVersion = "0.1.2rc1",
            actions = SettingsActions(),
        )
    }
}
