package com.guyu2233.ibukirpg.app.ui

import androidx.compose.animation.fadeIn
import androidx.compose.animation.fadeOut
import androidx.compose.animation.slideInHorizontally
import androidx.compose.animation.slideOutHorizontally
import androidx.compose.runtime.Composable
import androidx.lifecycle.createSavedStateHandle
import androidx.lifecycle.viewmodel.compose.viewModel
import androidx.navigation.compose.NavHost
import androidx.navigation.compose.composable
import androidx.navigation.compose.rememberNavController
import com.guyu2233.ibukirpg.app.IbukiApp
import com.guyu2233.ibukirpg.app.ui.game.GameScreen
import com.guyu2233.ibukirpg.app.ui.game.GameViewModel
import com.guyu2233.ibukirpg.app.ui.home.HomeScreen
import com.guyu2233.ibukirpg.app.ui.home.HomeViewModel
import com.guyu2233.ibukirpg.app.ui.home.CrashLogSource
import com.guyu2233.ibukirpg.app.crash.CrashReporter
import com.guyu2233.ibukirpg.app.ui.packs.PacksScreen
import com.guyu2233.ibukirpg.app.ui.packs.PacksViewModel
import com.guyu2233.ibukirpg.app.ui.saves.SavesScreen
import com.guyu2233.ibukirpg.app.ui.saves.SavesViewModel
import com.guyu2233.ibukirpg.app.ui.settings.SettingsScreen
import com.guyu2233.ibukirpg.app.ui.settings.SettingsViewModel

object Routes {
    const val HOME = "home"
    const val SAVES = "saves"
    const val GAME = "game"
    const val SETTINGS = "settings"
    const val PACKS = "packs"
}

@Composable
fun AppNavHost(app: IbukiApp) {
    val nav = rememberNavController()
    val toGame: () -> Unit = {
        nav.navigate(Routes.GAME) {
            popUpTo(Routes.HOME)
            launchSingleTop = true
        }
    }
    NavHost(
        navController = nav,
        startDestination = Routes.HOME,
        enterTransition = { slideInHorizontally { it / 4 } + fadeIn() },
        exitTransition = { fadeOut() },
        popEnterTransition = { fadeIn() },
        popExitTransition = { slideOutHorizontally { it / 4 } + fadeOut() },
    ) {
        composable(Routes.HOME) {
            val vm = viewModel {
                HomeViewModel(app.engine, app.settings, app.aiStartupReady, object : CrashLogSource {
                    override fun pending() = CrashReporter.pending(app)
                    override fun clear() = CrashReporter.clear(app)
                })
            }
            HomeScreen(
                vm = vm,
                onGameReady = toGame,
                onNewGame = { nav.navigate(Routes.PACKS) { launchSingleTop = true } },
                onSaves = { nav.navigate(Routes.SAVES) },
                onSettings = { nav.navigate(Routes.SETTINGS) },
            )
        }
        composable(Routes.SAVES) {
            val vm = viewModel { SavesViewModel(app.engine, app) }
            SavesScreen(vm = vm, onBack = { nav.popBackStack() }, onGameReady = toGame)
        }
        composable(Routes.PACKS) {
            val vm = viewModel { PacksViewModel(app.engine, app) }
            PacksScreen(vm = vm, onBack = { nav.popBackStack() }, onGameReady = toGame)
        }
        composable(Routes.GAME) {
            val vm = viewModel { GameViewModel(app.engine, createSavedStateHandle()).also { g -> g.onPromptsSaved = { p -> app.settings.savePrompts(p) } } }
            GameScreen(vm = vm, onBack = { nav.popBackStack(Routes.HOME, inclusive = false) })
        }
        composable(Routes.SETTINGS) {
            val vm = viewModel { SettingsViewModel(app.engine, app.settings, app.localAI, app.aiStartupReady) }
            SettingsScreen(vm = vm, onBack = { nav.popBackStack() })
        }
    }
}
