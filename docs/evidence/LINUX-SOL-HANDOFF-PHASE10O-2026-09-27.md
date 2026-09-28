# Phase 10O cancellation finding and interim correction

Observed 2026-09-27 in the same disposable Ubuntu GNOME Wayland account as
[Phase 10N](LINUX-SOL-HANDOFF-PHASE10N-2026-09-27.md). This is a preserved
interim build, superseded by Phase 10P. Everything remained local.

The Phase 10N installed GUI began restoring the 6.6 GB archive into a new
folder. Clicking Cancel around 8% stopped the copy, removed the staging folder
and left all three existing VM disks at their original inodes. The helper
window remained on “Cancelling setup...” indefinitely because it ignored the
home state sent by the launcher after the operation finished. See the
[stuck window](phase10n/restore-cancel-final2.png).

The first correction in `linux-ui/main.go` ignored progress during cancellation
and accepted a new home prompt once the launcher finished. Its table test in
`linux-ui/main_test.go` passed. The source was snapshotted at
`/home/bts/.cache/try-omarchy-phase10o-source`; the
[delta](phase10o/source-delta.tar.gz) SHA256 was
`52d16800388eb3d0d4bfe71bab4fe17653d8521c0946c8c2307b5487510a4665`.
The [manifest](phase10o/source-delta.json) and
[reconstruction check](phase10o/reconstruction-check.txt) cover 746 files.

The installed [interim bundle](phase10o/phase10o-cancel-interim.flatpak) had
SHA256 `c4505f4f1dd86fb24aaada01588044c4ba8fcf0696dabc0051cb664fa82b4589`
and OSTree
`646639af909cd0e287b8d74644977ebeeeb35a3d29d03f6f5675c6a5836f8f78`.
In the actual installed GUI, cancellation returned to a usable home with an
explicit preservation message: [result](phase10o/cancel-immediate.png).
The restore parent was empty and original disk inodes 1123315, 4980753 and
9699343 remained. Backup and recovery reopened after cancellation:
[menu](phase10o/recovery-after-cancel.png). A 10 byte invalid ZIP produced a
visible error and did not create output: [result](phase10o/invalid-archive-result.png).
The [installed identity](phase10o/installed-candidate.txt) records the same
three disk inodes and matched guest boot pair after both failure paths.

The interim build passed the [UI test](phase10o/ui-tests.log),
[full Linux race suite with QEMU 11.1.1](phase10o/compatible-suite.log),
Linux and Windows vet, Windows cross-build and Windows test compilation
([log](phase10o/cross-checks.log)). The full-suite log SHA256 is
`0b223080df7ef1c4f9f4a91a208c3f0332eaab9ad6f38112d816df02d54215d8`.
Native Windows CI was not run.

Further UI inspection found that the home actions worked but Close and Back
remained disabled. The helper had disabled that button on the Cancel click
and did not re-enable it when accepting the home prompt. The titlebar could
exit. This is a real end-user defect, so the Phase 10O bundle is not the
candidate. Phase 10P adds the button reset and repeats the installed path.
