from pathlib import Path
import yaml
ROOT=Path(__file__).resolve().parents[1]
def load_yaml(path):
    return yaml.safe_load(path.read_text())

def dynamics_config(stage: int) -> dict[str, object]:
    dynamics = load_yaml(ROOT / "genesis" / "dynamics.yaml")
    if stage == 3:
        return {}
    required = {
        "concern.base_drive": dynamics.get("concern", {}).get("base_drive"),
        "concern.urgency_weight": dynamics.get("concern", {}).get("urgency_weight"),
        "attention.novelty_weight": dynamics.get("attention", {}).get("novelty_weight"),
        "attention.trigger_threshold": dynamics.get("attention", {}).get("trigger_threshold"),
        "attention.revisit_seconds": dynamics.get("attention", {}).get("revisit_seconds"),
        "attention.maximum_idle_seconds": dynamics.get("attention", {}).get("maximum_idle_seconds"),
        "difference.accumulation_decay_rate": dynamics.get("difference", {}).get("accumulation_decay_rate"),
        "difference.learning_rate": dynamics.get("difference", {}).get("learning_rate"),
        "value_field.idle_growth": dynamics.get("value_field", {}).get("idle_growth"),
        "exploration.unknown_growth": dynamics.get("exploration", {}).get("unknown_growth"),
        "exploration.relief": dynamics.get("exploration", {}).get("relief"),
    }
    missing = [key for key, value in required.items() if value is None]
    if missing:
        raise RuntimeError("stage-four dynamics are not frozen: " + ", ".join(missing))
    result = {
        "affect_return_rate": dynamics["affect"]["return_rate"],
        "concern_base_drive": required["concern.base_drive"],
        "concern_urgency_weight": required["concern.urgency_weight"],
        "concern_growth_gain": dynamics["concern"]["growth_gain"],
        "concern_resolution_gain": dynamics["concern"]["resolution_gain"],
        "concern_natural_decay_rate": dynamics["concern"]["natural_decay_rate"],
        "attention_affect_weight": dynamics["attention"]["affect_weight"],
        "attention_value_weight": dynamics["attention"]["value_weight"],
        "attention_novelty_weight": required["attention.novelty_weight"],
        "attention_cost_weight": dynamics["attention"]["resource_cost_weight"],
        "attention_threshold": required["attention.trigger_threshold"],
        "attention_candidate_limit": 3,
        "attention_revisit_seconds": required["attention.revisit_seconds"],
        "attention_maximum_idle_seconds": required["attention.maximum_idle_seconds"],
        "difference_decay_rate": required["difference.accumulation_decay_rate"],
        "difference_learning_rate": required["difference.learning_rate"],
        "value_idle_growth": required["value_field.idle_growth"],
        "exploration_unknown_growth": required["exploration.unknown_growth"],
        "exploration_relief": required["exploration.relief"],
        "value_activation_gain": dynamics["value_field"]["activation_gain"],
        "value_activation_return_rate": dynamics["value_field"]["activation_return_rate"],
        "value_satiation_gain": dynamics["value_field"]["satiation_gain"],
        "value_satiation_return_rate": dynamics["value_field"]["satiation_return_rate"],
        "value_orientation_gain": dynamics["value_field"]["orientation_gain"],
    }
    if stage >= 5:
        integrity = dynamics.get("integrity", {})
        required_integrity = ("persistence", "gap_gain", "repair_gain", "mirror_threshold")
        missing_integrity = [name for name in required_integrity if integrity.get(name) is None]
        if missing_integrity:
            raise RuntimeError("stage-five integrity dynamics are not frozen: " + ", ".join(missing_integrity))
        result.update(
            {
                "integrity_persistence": integrity["persistence"],
                "integrity_gap_gain": integrity["gap_gain"],
                "integrity_repair_gain": integrity["repair_gain"],
                "integrity_mirror_threshold": integrity["mirror_threshold"],
            }
        )
    return result

def seed_config() -> dict[str, object]:
    seed = load_yaml(ROOT / "genesis" / "seed.yaml")
    identity = seed["identity"]
    values = seed["value_priors"]
    return {
        "name": identity["name"],
        "gender": identity["gender"],
        "age": identity["age"],
        "life_form": identity["life_form"],
        "value_orientation": values["orientation"],
        "reality_integrity_sensitivity": values["reality_integrity_sensitivity"],
        "semantic_text": (ROOT / "genesis" / "seed.md").read_text(encoding="utf-8"),
    }
