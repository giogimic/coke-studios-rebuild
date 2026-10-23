/* Coke Studios Cache Finder: looks through an old computer for files the
 * game left behind, so lost parts of it can be recovered. */
#include "finder.h"

static void banner(void)
{
    say("\n"
        "  ==============================================================\n"
        "    Coke Studios Cache Finder %s\n"
        "    Helping to recover the lost files of Coke Studios,\n"
        "    Coke Music and MyCoke (2002-2007)\n"
        "  ==============================================================\n\n",
        CCF_VERSION);
}

static void usage(void)
{
    banner();
    say("Run it with no options to be asked what to do.\n\n"
        "  CokeCacheFinder [options] [folder | drive | disk image | device ...]\n\n"
        "  (nothing)      search this whole computer\n"
        "  folder/drive   search just there (or drag it onto the program)\n"
        "  disk image     search inside a raw image (.img .dd .bin .001 ...)\n"
        "  device         search a raw disk, e.g. \\\\.\\E: or /dev/sdb (admin/root)\n\n"
        "  --deleted      also search the raw disks for deleted files (slow, admin)\n"
        "  --out FOLDER   where to put the results (default: next to the program)\n"
        "  --no-zip       leave the results as a folder only\n"
        "  --yes          ask no questions and do not wait at the end\n\n"
        "Nothing is ever sent anywhere. The results are a folder and a .zip that\n"
        "you can choose to send to the project:\n  %s\n",
        CCF_PROJECT_URL);
}

static void wait_enter(void)
{
    char line[8];
    say("\nPress Enter to close this window.");
    con_read_line(line, sizeof line);
}

static char *ask(const char *question)
{
    char line[1024];
    char *p, *e;
    say("%s", question);
    if (con_read_line(line, sizeof line) < 0) return NULL;
    p = line;
    while (*p == ' ' || *p == '\t' || *p == '"' || *p == '\'') p++;
    e = p + str_len(p);
    while (e > p && (e[-1] == ' ' || e[-1] == '\t' || e[-1] == '"' || e[-1] == '\'')) e--;
    return str_ndup(p, (size_t)(e - p));
}

static void add_place(Scan *s, const char *place)
{
    char **v = (char **)mem_realloc(s->places, (size_t)(s->nplaces + 1) * sizeof(char *));
    if (!v) return;
    s->places = v;
    s->places[s->nplaces++] = redact(place);
}

static int menu(int *deleted, char ***paths, int *npaths)
{
    char *c;
    say("This looks through this computer for files Coke Studios left behind -\n"
        "browser caches, Shockwave's folders, downloads and old backups - and\n"
        "copies any it finds into one .zip file.\n\n"
        "It only READS your files. Nothing is sent anywhere: at the end you\n"
        "decide whether to send the .zip to the project.\n\n"
        "  1  Search this computer (recommended)\n"
        "  2  Search this computer, and also look for DELETED files\n"
        "     (much slower; needs administrator rights)\n"
        "  3  Search one folder, drive or disk image\n"
        "  Q  Quit\n\n");
    for (;;) {
        c = ask("Your choice [1]: ");
        if (!c) return -1;
        if (!c[0] || str_eq(c, "1")) break;
        if (str_eq(c, "2")) {
            *deleted = 1;
            break;
        }
        if (str_eq(c, "3")) {
            char *p;
            mem_free(c);
            say("\nType the folder, drive (like E:\\) or disk image to search, or drag it\n"
                "onto this window, then press Enter.\n");
            p = ask("> ");
            if (!p || !p[0]) {
                mem_free(p);
                return -1;
            }
            *paths = (char **)mem_alloc(sizeof(char *));
            (*paths)[0] = p;
            *npaths = 1;
            return 0;
        }
        if (str_ieq(c, "q")) {
            mem_free(c);
            return -1;
        }
        mem_free(c);
    }
    mem_free(c);
    return 0;
}

/* Reading a raw disk needs administrator rights. Find out before the long
 * search starts, and if they are missing ask Windows to start a copy that
 * has them, so nobody waits an hour only to be told to start again. */
static int deleted_allowed(int may_elevate, int *relaunched)
{
    int n;
    char **vols;
    PFile *f;
    if (!raw_supported()) {
        say("Searching deleted files is not possible on this system from here. If you\n"
            "can, connect this hard drive to a newer computer and search it there.\n"
            "Searching the normal files now.\n");
        return 0;
    }
    vols = raw_volumes(&n);
    if (!n) return 0;
    f = f_open_read(vols[0]);
    if (f) {
        f_close(f);
        return 1;
    }
    if (f_last_error_denied() && may_elevate) {
        say("Searching deleted files needs administrator rights. Windows will ask\n"
            "for permission, and the search will carry on in a new window.\n");
        if (relaunch_elevated("--deleted --elevated")) {
            *relaunched = 1;
            return 0;
        }
    }
    say("Windows did not allow reading the raw disk, so deleted files will not be\n"
        "searched. (Right-click the program and choose \"Run as administrator\" to\n"
        "include them.) Searching the normal files now.\n");
    return 0;
}

static void search_deleted(Scan *s)
{
    int n, i;
    char **vols = raw_volumes(&n);
    for (i = 0; i < n && !s->stopped; i++) {
        int r;
        say("\nSearching the raw disk %s for deleted files. This takes a while;\n"
            "press Ctrl+C to stop early - what was found so far is kept.\n", vols[i]);
        r = scan_raw(s, vols[i]);
        status_clear(s);
        if (r == -2) {
            say("  Windows did not allow reading %s.\n", vols[i]);
            continue;
        }
        if (r == 0) {
            char *p = fmt("%s (raw disk, for deleted files)", vols[i]);
            add_place(s, p);
            mem_free(p);
        }
    }
}

int app_main(int argc, char **argv)
{
    Scan s;
    char **paths = NULL, *zip = NULL, *out = NULL;
    int npaths = 0, i, deleted = 0, batch = 0, no_zip = 0, elevated = 0, menu_shown = 0;
    int pause, relaunched = 0, new_coke = 0, old_coke = 0, other = 0, r;
    Date now;

    memset(&s, 0, sizeof s);
    for (i = 1; i < argc; i++) {
        const char *a = argv[i];
        if (str_eq(a, "--help") || str_eq(a, "-h") || str_eq(a, "/?") || str_eq(a, "-?")) {
            usage();
            return 0;
        } else if (str_eq(a, "--version")) {
            say("%s\n", CCF_VERSION);
            return 0;
        } else if (str_eq(a, "--deleted")) {
            deleted = 1;
        } else if (str_eq(a, "--elevated")) {
            elevated = 1;
        } else if (str_eq(a, "--no-zip")) {
            no_zip = 1;
        } else if (str_eq(a, "--yes")) {
            batch = 1;
        } else if (str_eq(a, "--out") && i + 1 < argc) {
            out = argv[++i];
        } else if (str_eq(a, "--make-known") && i + 2 < argc) {
            return make_known(argv[i + 1], argv + i + 2, argc - i - 2);
        } else if (str_eq(a, "--classify") && i + 1 < argc) {
            s.dry_run = s.verbose = 1;
            for (i++; i < argc; i++) scan_path(&s, argv[i]);
            return 0;
        } else {
            char **v = (char **)mem_realloc(paths, (size_t)(npaths + 1) * sizeof(char *));
            if (!v) return 1;
            paths = v;
            paths[npaths++] = str_dup(a);
        }
    }

    banner();
    if (!npaths && !deleted && !batch && !elevated && con_interactive()) {
        menu_shown = 1;
        if (menu(&deleted, &paths, &npaths)) return 0;
    }
    pause = !batch && (menu_shown || elevated || con_owned());
    if (deleted) {
        deleted = deleted_allowed(!elevated && !batch, &relaunched);
        if (relaunched) return 0;
    }

    s.out_base = str_dup(out ? out : exe_dir());
    date_now(&now);
    {
        char d[11];
        date_str(&now, d);
        s.out_name = fmt("CokeStudiosFinds-%s-%04X", d, random_seed() & 0xFFFF);
    }
    s.deleted_mode = deleted;
    s.start_ms = ms_now();
    say("\nSearching. Anything found is listed below as it turns up.\n"
        "Press Ctrl+C to stop early - what was found so far is kept.\n\n");

    if (npaths) {
        for (i = 0; i < npaths && !s.stopped; i++) {
            say("Searching %s ...\n", paths[i]);
            scan_path(&s, paths[i]);
            add_place(&s, paths[i]);
        }
    } else {
        int n;
        char **roots = search_roots(&n);
        for (i = 0; i < n && !s.stopped; i++) {
            status_clear(&s);
            say("Searching %s ...\n", roots[i]);
            scan_tree(&s, roots[i]);
            add_place(&s, roots[i]);
        }
    }
    status_clear(&s);
    if (deleted && !s.stopped) search_deleted(&s);
    status_clear(&s);

    for (i = 0; i < s.nf; i++) {
        if (s.f[i].known) old_coke++;
        else if (s.f[i].cat == CAT_COKE) new_coke++;
        else other++;
    }
    say("\nDone. Checked %N file%s in %u min %u s.\n\n", s.files_checked, s.files_checked == 1 ? "" : "s", (ms_now() - s.start_ms) / 60000,
        (ms_now() - s.start_ms) / 1000 % 60);
    r = write_results(&s, !no_zip, &zip);
    if (r == 1) {
        say("No Coke Studios files or traces were found here. Thank you for looking!\n\n"
            "If this computer was used for Coke Studios and has been cleaned up since,\n"
            "the files may still be on the disk as deleted data: run this again and\n"
            "choose option 2.\n");
    } else if (r < 0) {
        say("Something was found, but the results could not be saved (is the disk full\n"
            "or read-only?). Try again with --out pointing at a folder you can write to.\n");
    } else {
        if (new_coke) say("  %d NEW Coke Studios file%s - the project does not have %s yet!\n", new_coke, new_coke == 1 ? "" : "s", new_coke == 1 ? "it" : "them");
        if (old_coke) say("  %d Coke Studios file%s the archive already has (still useful for dating)\n", old_coke, old_coke == 1 ? "" : "s");
        if (other) say("  %d other Shockwave file%s, possibly from the game's first version\n", other, other == 1 ? "" : "s");
        if (s.urls.n) say("  %d old Coke Studios web address%s\n", s.urls.n, s.urls.n == 1 ? "" : "es");
        if (s.notes.n) say("  %d other sign%s the game was played here\n", s.notes.n, s.notes.n == 1 ? "" : "s");
        if (s.named.n) say("  %d file%s with \"coke\" in the name, listed for you to look at\n", s.named.n, s.named.n == 1 ? "" : "s");
        say("\nThe results are in:\n  %s\n", zip ? zip : s.out_dir);
        say("\nPlease send %s to the project - see how at:\n  %s\n", zip ? "this .zip file" : "that folder", CCF_PROJECT_URL);
        say("It holds only the files listed in report.txt inside it. Open it first if\n"
            "you would like to check.\n");
        if (pause) open_folder(s.out_base);
    }
    if (pause) wait_enter();
    return 0;
}
