# 0004 — Licenca Apache-2.0

- **Status:** Accepted 2026-10-05. `gw` se objavljuje pod Apache License 2.0; tekst je u `LICENSE`.
- **Date:** 2026-10-05
- **Supersedes:** — / **Superseded by:** —
- **Owner:** Darko
- **Related:** `gopro-fork-review-and-handover.md` §1 (zašto se ne kopira kod iz forka), §6 (licenca je bila otvoreno pitanje); `packaging-and-release.md`

## Context

Projekat ide ka javnom izdanju i paketima za distribucije, a do sada nije imao licencu. Bez licence niko, uključujući i distribucije, nema pravo da ga deli ili menja. Darko je predložio MPL-2.0 ili Apache-2.0.

`gw` je program za krajnjeg korisnika, a ne biblioteka. Kod iz forka `gopro_as_webcam_on_linux`, koji je pod Apache-2.0, nije kopiran (predajna beleška §1), pa licenca forka ne obavezuje izbor.

## Decision

1. Licenca je **Apache License 2.0**, SPDX identifikator `Apache-2.0`.
2. U korenu repoa je `LICENSE` sa kanonskim tekstom sa `https://www.apache.org/licenses/LICENSE-2.0.txt` (sha256 `cfc7749b96f63bd31c3c42b5c471bf756814053e847c10f3eb003417bc523d30`, preuzet 2026-10-05). Tekst je isti kao `/usr/share/licenses/spdx/Apache-2.0.txt` iz Arch paketa `licenses` kad se zanemari prelom redova.
3. `NOTICE` fajl se za sada ne dodaje; nije obavezan, a i bez njega licenca važi u potpunosti.
4. Paketi nose SPDX identifikator u polju za licencu, a tekst licence instaliraju tamo gde distribucija to traži (`packaging-and-release.md`).

## Consequences

**Positive**

- Najčešća licenca u Go ekosistemu; distribucije, firme i saradnici je dobro poznaju.
- Izričita patentna dozvola od svakog ko doprinese kod.
- Nema obaveza za one koji `gw` menjaju i dele, osim da sačuvaju obaveštenje o licenci i označe izmene.

**Negative**

- Neko može da napravi zatvorenu izmenjenu verziju i da je ne objavi.

## Alternatives considered

- **MPL-2.0:** copyleft na nivou fajla; ko deli izmenjen `gw` mora da objavi izmenjene fajlove. Darko je izabrao Apache-2.0.
- **GPL:** jači copyleft nego što je potrebno za ovakav alat.
- **MIT:** slično permisivna, ali bez izričite patentne dozvole.
