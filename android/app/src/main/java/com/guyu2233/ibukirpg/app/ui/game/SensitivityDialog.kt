package com.guyu2233.ibukirpg.app.ui.game

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.SegmentedButton
import androidx.compose.material3.SegmentedButtonDefaults
import androidx.compose.material3.SingleChoiceSegmentedButtonRow
import androidx.compose.material3.Switch
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.unit.dp
import com.guyu2233.ibukirpg.app.data.PromptSettingsV1
import com.guyu2233.ibukirpg.app.data.SensitivityV1

private val levels = listOf("off" to "关", "low" to "低", "mid" to "中", "high" to "高")

/** 提示灵敏度：三类提示各自的灵敏度 + “只通知”模式 + 自动检查点。 */
@Composable
fun SensitivityDialog(p: PromptSettingsV1, onDismiss: () -> Unit, onSave: (PromptSettingsV1) -> Unit) {
    var v by remember(p) { mutableStateOf(p) }
    AlertDialog(
        onDismissRequest = onDismiss,
        title = { Text("提示灵敏度") },
        text = { SensitivityContent(v) { v = it } },
        confirmButton = { TextButton(onClick = { onSave(v) }) { Text("保存") } },
        dismissButton = { TextButton(onClick = onDismiss) { Text("取消") } },
    )
}

@Composable
fun SensitivityContent(v: PromptSettingsV1, onChange: (PromptSettingsV1) -> Unit) {
    Column(Modifier.verticalScroll(rememberScrollState()), verticalArrangement = Arrangement.spacedBy(14.dp)) {
        Text(
            "AI 改动世界时，影响越大越可能停下来问你。灵敏度越高，越小的改动也会提示；“关”= 从不提示。",
            style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant,
        )
        SensRow("偏离设定", "改变了故事包的既定设定 / 关键事件", v.loreDeviation) { onChange(v.copy(loreDeviation = it)) }
        SensRow("重要角色死亡", "关系网里的重要人物死亡或退场", v.majorDeath) { onChange(v.copy(majorDeath = it)) }
        SensRow("重大剧情影响", "揭示隐藏真相、势力格局变化等", v.storyImpact) { onChange(v.copy(storyImpact = it)) }
        SwitchRow("提示前自动创建检查点", v.autoCheckpoint) { onChange(v.copy(autoCheckpoint = it)) }
        SwitchRow("每个游戏日自动检查点", v.dailyCheckpoint) { onChange(v.copy(dailyCheckpoint = it)) }
    }
}

@Composable
private fun SensRow(title: String, desc: String, s: SensitivityV1, onChange: (SensitivityV1) -> Unit) {
    Column(verticalArrangement = Arrangement.spacedBy(6.dp)) {
        Text(title, style = MaterialTheme.typography.titleSmall)
        Text(desc, style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
        SingleChoiceSegmentedButtonRow(Modifier.fillMaxWidth()) {
            levels.forEachIndexed { i, (id, label) ->
                SegmentedButton(
                    selected = s.level == id,
                    onClick = { onChange(s.copy(level = id)) },
                    shape = SegmentedButtonDefaults.itemShape(i, levels.size),
                ) { Text(label) }
            }
        }
        SwitchRow("只通知，不打断", s.mode == "notify", enabled = s.level != "off") { onChange(s.copy(mode = if (it) "notify" else "modal")) }
    }
}

@Composable
private fun SwitchRow(label: String, checked: Boolean, enabled: Boolean = true, onChange: (Boolean) -> Unit) {
    Row(Modifier.fillMaxWidth().padding(vertical = 2.dp), verticalAlignment = Alignment.CenterVertically) {
        Text(label, style = MaterialTheme.typography.bodyMedium, modifier = Modifier.weight(1f))
        Switch(checked = checked, onCheckedChange = onChange, enabled = enabled)
    }
}
