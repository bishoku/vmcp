import importlib.util
from pathlib import Path
import unittest

spec = importlib.util.spec_from_file_location('prepare_wiki', Path(__file__).with_name('prepare-wiki.py'))
module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(module)


class WikiLinksTest(unittest.TestCase):
    def render(self, text):
        root = Path(__file__).resolve().parent.parent
        return module.render(text, root / 'docs/Home.md', root, 'https://github.com/bishoku/vmcp')

    def test_wiki_links_and_anchors(self):
        self.assertEqual(self.render('[a](Development.md#setup) [b](Home.md)'), '[a](Development#setup) [b](Home)')

    def test_repository_files_and_directories(self):
        self.assertEqual(self.render('[a](../README.md#roadmap) [b](../core/)'), '[a](https://github.com/bishoku/vmcp/blob/main/README.md#roadmap) [b](https://github.com/bishoku/vmcp/tree/main/core)')

    def test_external_links_and_anchors(self):
        text = '[a](https://example.com/a.md) [b](#setup) [c](mailto:a@example.com)'
        self.assertEqual(self.render(text), text)

    def test_fenced_examples_are_preserved(self):
        for marker in ('```', '~~~~'):
            text = f'{marker}md\n[a](Home.md)\n{marker}\n[a](Home.md)\n'
            self.assertEqual(self.render(text), f'{marker}md\n[a](Home.md)\n{marker}\n[a](Home)\n')


if __name__ == '__main__':
    unittest.main()
