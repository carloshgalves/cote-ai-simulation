"""The population prior: two latent factors and a gaussian copula.

`physical-model.md` §5.5 and the research §2. Independent marginals are wrong in
a way you can see — they populate the joint tail with the student who is at once
the heaviest, the fastest and the best at the shuttle run (failure mode F7). The
prior is therefore a general-fitness factor plus a build factor plus a residual
per dimension, applied as a gaussian copula so that each marginal in
`population-prior.yaml` is preserved **exactly**:

    z_d = a_d·f_general + b_d·f_build + r_d·e_d ,  r_d = sqrt(1 − a_d² − b_d²)
    x_d = ppf_d( Φ(z_d) )

The whole file is `[INT]`: the structure is settled, the numbers are not (S1,
S2). `evidence_sufficiency` says so in the file's own header, which is where the
model asks for it to be said — not only on the character sheets.

Percentile is computed here, from the marginal, as a **derived view**
(`percentile_of`). It is never stored (invariant 9).
"""

from __future__ import annotations

import hashlib
import json
import math
from collections.abc import Mapping, Sequence
from pathlib import Path
from typing import Any, Literal

import numpy as np
from pydantic import BaseModel, ConfigDict, field_validator, model_validator

from .modelfile import PHYSICAL_MODELS_DIR, ModelFileError, load_model_file
from .types import (
    DIMENSION_UNITS,
    CapacityProfile,
    Dimension,
    DimensionValue,
    Sex,
    freeze_mapping,
)

__all__ = [
    "Sex",
    "Marginal",
    "Loading",
    "PopulationPrior",
    "PriorError",
    "POPULATION_PRIOR_PATH",
    "FACTORS",
]

POPULATION_PRIOR_PATH = PHYSICAL_MODELS_DIR / "population-prior.yaml"

SEXES: tuple[Sex, ...] = ("male", "female")

#: Order matters: it fixes which draw of the substream feeds which factor, so it
#: is part of the reproducibility contract and not a presentation choice.
FACTORS: tuple[str, ...] = ("general_fitness", "build")

_SQRT2 = math.sqrt(2.0)


class PriorError(ValueError):
    """The prior file is unusable. Loading it half-right is worse than not at all."""


def _phi_scalar(z: float) -> float:
    """Standard normal CDF, without a scipy dependency."""
    return 0.5 * (1.0 + math.erf(z / _SQRT2))


def _phi(z: np.ndarray) -> np.ndarray:
    return np.asarray([_phi_scalar(float(value)) for value in np.ravel(z)]).reshape(np.shape(z))


def _probit(u: np.ndarray) -> np.ndarray:
    """Inverse standard normal CDF (Acklam's rational approximation, ~1e-9)."""
    u = np.asarray(u, dtype=float)
    if np.any((u <= 0.0) | (u >= 1.0)):
        raise ValueError("probit is defined on the open interval (0, 1)")
    a = (-3.969683028665376e01, 2.209460984245205e02, -2.759285104469687e02,
         1.383577518672690e02, -3.066479806614716e01, 2.506628277459239e00)
    b = (-5.447609879822406e01, 1.615858368580409e02, -1.556989798598866e02,
         6.680131188771972e01, -1.328068155288572e01)
    c = (-7.784894002430293e-03, -3.223964580411365e-01, -2.400758277161838e00,
         -2.549732539343734e00, 4.374664141464968e00, 2.938163982698783e00)
    d = (7.784695709041462e-03, 3.224671290700398e-01, 2.445134137142996e00,
         3.754408661907416e00)
    low, high = 0.02425, 1.0 - 0.02425
    out = np.empty_like(u)

    lower = u < low
    if np.any(lower):
        q = np.sqrt(-2.0 * np.log(u[lower]))
        out[lower] = (((((c[0] * q + c[1]) * q + c[2]) * q + c[3]) * q + c[4]) * q + c[5]) / (
            (((d[0] * q + d[1]) * q + d[2]) * q + d[3]) * q + 1.0
        )
    upper = u > high
    if np.any(upper):
        q = np.sqrt(-2.0 * np.log(1.0 - u[upper]))
        out[upper] = -(((((c[0] * q + c[1]) * q + c[2]) * q + c[3]) * q + c[4]) * q + c[5]) / (
            (((d[0] * q + d[1]) * q + d[2]) * q + d[3]) * q + 1.0
        )
    middle = ~(lower | upper)
    if np.any(middle):
        q = u[middle] - 0.5
        r = q * q
        out[middle] = (((((a[0] * r + a[1]) * r + a[2]) * r + a[3]) * r + a[4]) * r + a[5]) * q / (
            ((((b[0] * r + b[1]) * r + b[2]) * r + b[3]) * r + b[4]) * r + 1.0
        )
    return out


class Marginal(BaseModel):
    """One dimension's marginal distribution, in that dimension's physical unit.

    Four families, each with a closed-form quantile function, which is what the
    copula needs and what makes "the marginal is preserved exactly" testable by
    comparing sample quantiles against `ppf`.
    """

    model_config = ConfigDict(frozen=True, extra="forbid")

    family: Literal["normal", "lognormal", "uniform", "logitnormal"]
    unit: str
    mean: float | None = None
    sd: float | None = None
    low: float | None = None
    high: float | None = None
    median: float | None = None
    logit_sd: float | None = None

    @field_validator("mean", "sd", "low", "high", "median", "logit_sd", mode="before")
    @classmethod
    def _numeric_parameter_is_not_boolean(cls, value: Any) -> Any:
        if isinstance(value, bool):
            raise ValueError("marginal numeric parameter must not be boolean")
        return value

    @model_validator(mode="after")
    def _parameters_match_family(self) -> Marginal:
        for name in ("mean", "sd", "low", "high", "median", "logit_sd"):
            value = getattr(self, name)
            if value is not None and not math.isfinite(value):
                raise ValueError(f"marginal parameter {name} must be finite")
        required = {
            "normal": ("mean", "sd"),
            "lognormal": ("mean", "sd"),
            "uniform": ("low", "high"),
            "logitnormal": ("median", "logit_sd"),
        }[self.family]
        for name in required:
            if getattr(self, name) is None:
                raise ValueError(f"{self.family} marginal requires {name}")
        if self.family in ("normal", "lognormal") and self.sd is not None and self.sd <= 0:
            raise ValueError("sd must be positive")
        if self.family == "lognormal" and self.mean is not None and self.mean <= 0:
            raise ValueError("a lognormal marginal needs a positive mean")
        if self.family == "uniform" and self.low is not None and self.high is not None:
            if self.high <= self.low:
                raise ValueError("uniform marginal needs high > low")
        if self.family == "logitnormal":
            if not 0.0 < float(self.median or 0.0) < 1.0:
                raise ValueError("a logitnormal median must lie strictly inside (0, 1)")
            if float(self.logit_sd or 0.0) <= 0.0:
                raise ValueError("logit_sd must be positive")
        return self

    @property
    def _log_parameters(self) -> tuple[float, float]:
        """Underlying normal parameters of a lognormal declared by physical mean/sd."""
        mean, sd = float(self.mean), float(self.sd)  # type: ignore[arg-type]
        log_sd = math.sqrt(math.log1p((sd / mean) ** 2))
        return math.log(mean) - 0.5 * log_sd**2, log_sd

    def ppf(self, u: np.ndarray | float) -> np.ndarray:
        """Quantile function: the copula's only entry point into this marginal."""
        u_array = np.atleast_1d(np.asarray(u, dtype=float))
        z = _probit(u_array)
        match self.family:
            case "normal":
                out = float(self.mean) + float(self.sd) * z  # type: ignore[arg-type]
            case "lognormal":
                log_mean, log_sd = self._log_parameters
                out = np.exp(log_mean + log_sd * z)
            case "uniform":
                out = float(self.low) + (float(self.high) - float(self.low)) * u_array  # type: ignore[arg-type]
            case "logitnormal":
                logit_median = math.log(float(self.median) / (1.0 - float(self.median)))  # type: ignore[arg-type]
                out = 1.0 / (1.0 + np.exp(-(logit_median + float(self.logit_sd) * z)))  # type: ignore[arg-type]
        return out

    def cdf(self, x: float) -> float:
        """Where a value falls in this marginal — the derived percentile view."""
        match self.family:
            case "normal":
                return _phi_scalar((x - float(self.mean)) / float(self.sd))  # type: ignore[arg-type]
            case "lognormal":
                if x <= 0:
                    return 0.0
                log_mean, log_sd = self._log_parameters
                return _phi_scalar((math.log(x) - log_mean) / log_sd)
            case "uniform":
                low, high = float(self.low), float(self.high)  # type: ignore[arg-type]
                return min(1.0, max(0.0, (x - low) / (high - low)))
            case "logitnormal":
                if x <= 0.0:
                    return 0.0
                if x >= 1.0:
                    return 1.0
                logit_median = math.log(float(self.median) / (1.0 - float(self.median)))  # type: ignore[arg-type]
                return _phi_scalar((math.log(x / (1.0 - x)) - logit_median) / float(self.logit_sd))  # type: ignore[arg-type]
        raise AssertionError("unreachable")  # pragma: no cover


class Loading(BaseModel):
    """One dimension's loadings on the two latent factors, plus its residual."""

    model_config = ConfigDict(frozen=True, extra="forbid")

    general_fitness: float
    build: float

    @field_validator("general_fitness", "build", mode="before")
    @classmethod
    def _numeric_parameter_is_not_boolean(cls, value: Any) -> Any:
        if isinstance(value, bool):
            raise ValueError("factor loading must not be boolean")
        return value

    @field_validator("general_fitness", "build", mode="after")
    @classmethod
    def _loading_is_finite(cls, value: float) -> float:
        if not math.isfinite(value):
            raise ValueError("factor loading must be finite")
        return value

    def residual_sd(self, minimum: float) -> float:
        communality = self.general_fitness**2 + self.build**2
        residual = 1.0 - communality
        if residual < minimum**2:
            raise PriorError(
                f"loadings {self.general_fitness}/{self.build} leave a residual sd below the "
                f"declared floor {minimum}: the dimension would be a near-deterministic "
                f"function of the two factors"
            )
        return math.sqrt(residual)


class PopulationPrior(BaseModel):
    """A loaded, validated `population-prior.yaml`."""

    model_config = ConfigDict(frozen=True, arbitrary_types_allowed=True)

    version: str
    status: str
    content_hash: str
    sex_ratio_male: float
    min_residual_sd: float
    marginals: Mapping[Sex, Mapping[Dimension, Marginal]]
    loadings: Mapping[Dimension, Loading]
    raw: Mapping[str, Any]

    @field_validator("marginals", "loadings", "raw", mode="after")
    @classmethod
    def _freeze_model_mappings(cls, value: Mapping[Any, Any]) -> Mapping[Any, Any]:
        return freeze_mapping(value)

    @field_validator("sex_ratio_male", mode="after")
    @classmethod
    def _sex_ratio_is_a_probability(cls, value: float) -> float:
        if not math.isfinite(value) or not 0.0 <= value <= 1.0:
            raise ValueError("sex_ratio_male must be finite and within [0, 1]")
        return value

    @field_validator("min_residual_sd", mode="after")
    @classmethod
    def _residual_floor_is_a_standard_deviation(cls, value: float) -> float:
        if not math.isfinite(value) or not 0.0 < value <= 1.0:
            raise ValueError("min_residual_sd must be finite and within (0, 1]")
        return value

    # ------------------------------------------------------------------ loading

    @classmethod
    def load(cls, path: str | Path = POPULATION_PRIOR_PATH) -> PopulationPrior:
        path = Path(path)
        try:
            data = load_model_file(path)
        except ModelFileError as error:
            raise PriorError(str(error)) from error
        if data.get("model_kind") != "POPULATION_PRIORS":
            raise PriorError(f"{path}: not a POPULATION_PRIORS file")
        return cls._from_data(data, path)

    @classmethod
    def _from_data(cls, data: Mapping[str, Any], path: Path) -> PopulationPrior:
        marginals: dict[Sex, dict[Dimension, Marginal]] = {}
        raw_marginals = data.get("marginals") or {}
        for sex in SEXES:
            per_sex = raw_marginals.get(sex)
            if not isinstance(per_sex, Mapping):
                raise PriorError(f"{path}: marginals.{sex} is missing")
            marginals[sex] = {}
            for dimension in Dimension:
                entry = per_sex.get(dimension.value)
                if entry is None:
                    raise PriorError(f"{path}: marginals.{sex} is missing dimension {dimension.value}")
                marginal = Marginal.model_validate(entry)
                expected_unit = DIMENSION_UNITS[dimension].unit
                if marginal.unit != expected_unit:
                    # Invariant 9: a dimension carries a physical unit, and the
                    # prior may not quietly change it under the engine.
                    raise PriorError(
                        f"{path}: marginals.{sex}.{dimension.value} declares unit {marginal.unit!r}, "
                        f"but the dimension is stored in {expected_unit!r}"
                    )
                marginals[sex][dimension] = marginal

        raw_loadings = data.get("loadings") or {}
        min_residual_sd = float(_sourced_number(data, ("copula", "min_residual_sd"), path))
        if not math.isfinite(min_residual_sd) or not 0.0 < min_residual_sd <= 1.0:
            raise PriorError(f"{path}: copula.min_residual_sd must be finite and within (0, 1]")
        loadings: dict[Dimension, Loading] = {}
        for dimension in Dimension:
            entry = raw_loadings.get(dimension.value)
            if entry is None:
                raise PriorError(f"{path}: loadings is missing dimension {dimension.value}")
            loading = Loading.model_validate(entry)
            loading.residual_sd(min_residual_sd)  # fail at load, not at first draw
            loadings[dimension] = loading

        declared_factors = tuple((data.get("copula") or {}).get("factors") or ())
        if declared_factors != FACTORS:
            raise PriorError(f"{path}: copula.factors must be {list(FACTORS)}, got {list(declared_factors)}")

        return cls(
            version=str(data["model_version"]),
            status=str(data["status"]),
            content_hash=content_hash(data),
            sex_ratio_male=float(_sourced_number(data, ("cohort", "sex_ratio_male"), path)),
            min_residual_sd=min_residual_sd,
            marginals=marginals,
            loadings=loadings,
            raw=data,
        )

    # ----------------------------------------------------------------- sampling

    def draw_sex(self, rng: np.random.Generator) -> Sex:
        """Sex is a cohort covariate, drawn in its own substream, never assigned.

        It is not a capacity dimension and does not come out of the copula; it
        conditions which marginals the copula uses.
        """
        return "male" if float(rng.random()) < self.sex_ratio_male else "female"

    def sample_matrix(self, rng: np.random.Generator, sex: Sex, size: int) -> dict[Dimension, np.ndarray]:
        """`size` bodies at once, in the physical unit of each dimension.

        The draw order is fixed — the two factors first, then one residual per
        dimension in the declaration order of `Dimension` — because a run that
        reproduces only when the dimensions happen to iterate the same way does
        not reproduce. `sample_profile` is this with `size == 1`, and consumes
        the stream identically.
        """
        if sex not in self.marginals:
            raise PriorError(f"unknown sex {sex!r}")
        if size < 1:
            raise ValueError("sample size must be at least 1")
        factors = rng.standard_normal((size, len(FACTORS)))
        residuals = rng.standard_normal((size, len(Dimension)))
        per_sex = self.marginals[sex]

        columns: dict[Dimension, np.ndarray] = {}
        for index, dimension in enumerate(Dimension):
            loading = self.loadings[dimension]
            z = (
                loading.general_fitness * factors[:, 0]
                + loading.build * factors[:, 1]
                + loading.residual_sd(self.min_residual_sd) * residuals[:, index]
            )
            u = np.clip(_phi(z), 1e-12, 1.0 - 1e-12)
            columns[dimension] = np.asarray(per_sex[dimension].ppf(u), dtype=float)
        return columns

    def sample_profile(self, rng: np.random.Generator, sex: Sex) -> CapacityProfile:
        """One body from the prior."""
        columns = self.sample_matrix(rng, sex, 1)
        return CapacityProfile(
            dimensions={
                dimension: DimensionValue(
                    value=float(values[0]), unit=DIMENSION_UNITS[dimension].unit
                )
                for dimension, values in columns.items()
            }
        )

    # -------------------------------------------------------------- derived view

    def percentile_of(self, dimension: Dimension, value: float, sex: Sex) -> float:
        """Cohort percentile as a **derived view** (invariant 9), never storage.

        Read in the unit the dimension is stored in: for `reaction_time` and
        `recovery_rate`, where lower is better, a high percentile means a high
        number and therefore a worse body. Orientation is the caller's business,
        because the cohort's own distribution has no opinion about it.
        """
        return 100.0 * self.marginals[sex][dimension].cdf(value)

    def z_score_of(self, dimension: Dimension, value: float, sex: Sex) -> float:
        """The value's position on the copula scale, i.e. the `z` that produced it.

        The inverse of the transform `sample_matrix` applies. Correlations across
        a mixed-sex cohort are measured here rather than on raw units: on raw
        units the difference between two marginals would show up as structure.
        """
        u = min(max(self.marginals[sex][dimension].cdf(value), 1e-9), 1.0 - 1e-9)
        return float(_probit(np.asarray([u]))[0])

    def marginal(self, dimension: Dimension, sex: Sex) -> Marginal:
        return self.marginals[sex][dimension]


def _sourced_number(data: Mapping[str, Any], path_keys: Sequence[str], path: Path) -> float:
    """Read `a.b` where the leaf is a `{source, value}` block or a bare number."""
    node: Any = data
    for key in path_keys:
        if not isinstance(node, Mapping) or key not in node:
            raise PriorError(f"{path}: missing {'.'.join(path_keys)}")
        node = node[key]
    if isinstance(node, Mapping):
        if "value" not in node:
            raise PriorError(f"{path}: {'.'.join(path_keys)} has no `value`")
        node = node["value"]
    if not isinstance(node, (int, float)) or isinstance(node, bool):
        raise PriorError(f"{path}: {'.'.join(path_keys)} must be a number")
    return float(node)


def content_hash(data: Mapping[str, Any]) -> str:
    """sha256 of the file's content, so a run records *which* prior it drew from."""
    canonical = json.dumps(data, sort_keys=True, ensure_ascii=False, default=str)
    return hashlib.sha256(canonical.encode("utf-8")).hexdigest()
