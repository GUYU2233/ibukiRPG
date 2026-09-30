package com.guyu2233.ibukirpg.app.ui.rpg

import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.Stable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import com.guyu2233.ibukirpg.app.data.CardV1
import com.guyu2233.ibukirpg.app.data.QuickActionV1

/** 介绍卡 / 机甲卡对话框的打开状态。 */
@Stable
class RpgDialogState(card: CardV1? = null, mech: String? = null, cardId: String? = null) {
    var card by mutableStateOf(card)
    var cardId by mutableStateOf(cardId)
    var mech by mutableStateOf(mech)

    fun openCard(c: CardV1) {
        if (c.kind == "mech" && c.known) mech = c.id else card = c
    }
}

@Composable
fun rememberRpgDialogState(card: CardV1? = null, mech: String? = null) = remember { RpgDialogState(card, mech) }

/** 统一的对话框宿主：卡片对话框、按 id 加载的卡片、全屏机甲卡。 */
@Composable
fun RpgDialogHost(state: RpgDialogState, onAction: (QuickActionV1, String) -> Unit, refreshKey: Any? = null) {
    val data = LocalRpgData.current
    state.cardId?.let { id ->
        LaunchedEffect(id) {
            val c = runCatching { data.card(id) }.getOrNull()
            state.cardId = null
            if (c != null) state.openCard(c)
        }
    }
    state.card?.let { c -> CardDialog(c, onDismiss = { state.card = null }, onOpenMech = { state.card = null; state.mech = it }) }
    state.mech?.let { id ->
        MechCardDialog(id, onDismiss = { state.mech = null }, onAction = { qa -> onAction(qa, qa.label ?: "") }, refreshKey = refreshKey)
    }
}
